package gatewayws

import (
	"context"
	"testing"
	"time"

	"cdr.dev/slog/sloggers/slogtest"
	"golang.org/x/time/rate"
)

// When the writer goroutine exits (e.g. a broken socket fails writeOp), it must
// cancel the connection context. Otherwise the read loop and sendHeartbeats can
// block forever on the unbuffered prioch send in writeHeartbeat — nothing drains
// prioch and nothing tears the connection down. Observed in production as shards
// frozen at "handle internal event " (empty type = the heartbeat op) with a
// stuck seq, unrecoverable even by ForceIdentify.
func TestWriterCancelsConnectionWhenItExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		ctx:    ctx,
		cancel: cancel,
		log:    slogtest.Make(t, nil),
		// burst 0 => rl.Wait(ctx) returns an error immediately, driving writer()
		// down its abnormal-exit path without needing a real websocket.
		rl:     rate.NewLimiter(rate.Every(time.Hour), 0),
		wch:    make(chan *Op, 1),
		prioch: make(chan *Op, 1),
		authed: true,
	}
	s.prioch <- &Op{Op: 1} // give writer a message so it advances to rl.Wait and exits

	done := make(chan struct{})
	go func() { s.writer(); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("writer did not return")
	}

	select {
	case <-s.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("writer exited without cancelling the connection context; the read loop can deadlock on prioch")
	}
}

// writeHeartbeat must not block forever when no writer is draining prioch. Once
// the connection context is cancelled (writer death, ForceIdentify, or the
// heartbeat watchdog) the send must abort so the read loop can unwind and
// reconnect.
func TestWriteHeartbeatUnblocksWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		ctx:    ctx,
		cancel: cancel,
		log:    slogtest.Make(t, nil),
		prioch: make(chan *Op), // unbuffered, no receiver: a bare send would wedge
	}
	cancel() // connection is being torn down

	done := make(chan struct{})
	go func() {
		s.writeHeartbeat()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("writeHeartbeat blocked on prioch after context cancel (deadlock)")
	}
}

// assertUnblocksOnCancel runs a Session send method with no receiver draining
// prioch/wch and a cancelled connection context, and fails if it doesn't return
// promptly. A bare channel send would deadlock here exactly as writeHeartbeat
// did; every gateway-initiated send must abort on ctx.Done instead.
func assertUnblocksOnCancel(t *testing.T, name string, run func(*Session)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		ctx:    ctx,
		cancel: cancel,
		log:    slogtest.Make(t, nil),
		prioch: make(chan *Op),
		wch:    make(chan *Op),
	}
	cancel()

	done := make(chan struct{})
	go func() {
		run(s)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s blocked on channel send after context cancel (deadlock)", name)
	}
}

func TestWriteIdentifyUnblocksOnCancel(t *testing.T) {
	assertUnblocksOnCancel(t, "writeIdentify", (*Session).writeIdentify)
}

func TestWriteResumeUnblocksOnCancel(t *testing.T) {
	assertUnblocksOnCancel(t, "writeResume", (*Session).writeResume)
}

func TestRequestGuildMembersUnblocksOnCancel(t *testing.T) {
	assertUnblocksOnCancel(t, "RequestGuildMembers", func(s *Session) { s.RequestGuildMembers(123) })
}

// heartbeatStale is the watchdog's death signal: a heartbeat we sent has gone
// unacked for at least one interval. The original condition
// (lastAck.Sub(lastHB) >= interval) was inverted — once ACKs stop, lastAck falls
// behind lastHB so the difference goes negative and the check can never fire,
// leaving a zombie connection (writes still accepted, no ACKs) wedged forever.
func TestHeartbeatStale(t *testing.T) {
	const interval = 40 * time.Second
	base := time.Now()

	cases := []struct {
		name           string
		lastHB, lastAck time.Time
		now            time.Time
		want           bool
	}{
		{"never sent a heartbeat", time.Time{}, time.Time{}, base, false},
		{"acked promptly", base, base.Add(time.Millisecond), base.Add(interval), false},
		{"unacked but within interval", base, base.Add(-2 * interval), base.Add(interval / 2), false},
		{"unacked past interval", base, base.Add(-2 * interval), base.Add(interval), true},
		{"first heartbeat never acked", base, time.Time{}, base.Add(interval), true},
	}

	for _, c := range cases {
		s := &Session{lastHB: c.lastHB, lastAck: c.lastAck, interval: interval}
		if got := s.heartbeatStale(c.now); got != c.want {
			t.Errorf("%s: heartbeatStale(%v) = %v, want %v", c.name, c.now, got, c.want)
		}
	}
}

// A Session is reused across reconnects, so heartbeat state from the previous
// connection must be cleared when a new one starts. If lastHB carried over while
// lastAck is reset to zero, heartbeatStale would read that old heartbeat as
// unacked and cancel the fresh connection on its first tick — a reconnect loop.
// resetHeartbeat must clear both timestamps together.
func TestResetHeartbeatPreventsStaleCancelAfterReconnect(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	s := &Session{
		lastHB:   old,                      // heartbeat sent on the previous connection
		lastAck:  old.Add(10 * time.Millisecond), // and acked then
		interval: 40 * time.Second,
	}

	s.resetHeartbeat()

	if !s.lastHB.IsZero() || !s.lastAck.IsZero() {
		t.Fatalf("resetHeartbeat left state: lastHB=%v lastAck=%v", s.lastHB, s.lastAck)
	}
	if s.heartbeatStale(time.Now()) {
		t.Fatal("heartbeatStale fired on a freshly reset connection; would cause a reconnect loop")
	}
}

// The fix must not break the happy path: with a receiver draining prioch and a
// live context, writeHeartbeat still delivers the heartbeat op.
func TestWriteHeartbeatDeliversWhenDrained(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Session{
		ctx:    ctx,
		cancel: cancel,
		log:    slogtest.Make(t, nil),
		prioch: make(chan *Op),
	}

	go s.writeHeartbeat()

	select {
	case op := <-s.prioch:
		if op.Op != 1 {
			t.Fatalf("heartbeat op = %d, want 1", op.Op)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("writeHeartbeat did not deliver to a draining receiver")
	}
}
