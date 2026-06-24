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
		log: slogtest.Make(t, nil),
		// burst 0 => rl.Wait(ctx) returns an error immediately, driving writer()
		// down its abnormal-exit path without needing a real websocket.
		rl: rate.NewLimiter(rate.Every(time.Hour), 0),
	}
	c := &conn{
		s:      s,
		ctx:    ctx,
		cancel: cancel,
		wch:    make(chan *Op, 1),
		prioch: make(chan *Op, 1),
	}
	c.authed.Store(true)
	c.prioch <- &Op{Op: 1} // give writer a message so it advances to rl.Wait and exits

	done := make(chan struct{})
	go func() { c.writer(); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("writer did not return")
	}

	select {
	case <-c.ctx.Done():
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
	s := &Session{log: slogtest.Make(t, nil)}
	c := &conn{
		s:      s,
		ctx:    ctx,
		cancel: cancel,
		prioch: make(chan *Op), // unbuffered, no receiver: a bare send would wedge
	}
	cancel() // connection is being torn down

	done := make(chan struct{})
	go func() {
		c.writeHeartbeat()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("writeHeartbeat blocked on prioch after context cancel (deadlock)")
	}
}

// assertUnblocksOnCancel runs a conn send method with no receiver draining
// prioch/wch and a cancelled connection context, and fails if it doesn't return
// promptly. A bare channel send would deadlock here exactly as writeHeartbeat
// did; every gateway-initiated send must abort on ctx.Done instead.
func assertUnblocksOnCancel(t *testing.T, name string, run func(*conn)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{log: slogtest.Make(t, nil)}
	c := &conn{
		s:      s,
		ctx:    ctx,
		cancel: cancel,
		prioch: make(chan *Op),
		wch:    make(chan *Op),
	}
	cancel()

	done := make(chan struct{})
	go func() {
		run(c)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s blocked on channel send after context cancel (deadlock)", name)
	}
}

func TestWriteIdentifyUnblocksOnCancel(t *testing.T) {
	assertUnblocksOnCancel(t, "writeIdentify", (*conn).writeIdentify)
}

func TestWriteResumeUnblocksOnCancel(t *testing.T) {
	assertUnblocksOnCancel(t, "writeResume", (*conn).writeResume)
}

func TestRequestGuildMembersUnblocksOnCancel(t *testing.T) {
	assertUnblocksOnCancel(t, "requestGuildMembersExternal", func(c *conn) { c.requestGuildMembersExternal(123) })
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
		name            string
		lastHB, lastAck time.Time
		now             time.Time
		want            bool
	}{
		{"never sent a heartbeat", time.Time{}, time.Time{}, base, false},
		{"acked promptly", base, base.Add(time.Millisecond), base.Add(interval), false},
		{"unacked but within interval", base, base.Add(-2 * interval), base.Add(interval / 2), false},
		{"unacked past interval", base, base.Add(-2 * interval), base.Add(interval), true},
		{"first heartbeat never acked", base, time.Time{}, base.Add(interval), true},
	}

	for _, tc := range cases {
		c := &conn{lastHB: tc.lastHB, lastAck: tc.lastAck, interval: interval}
		if got := c.heartbeatStale(tc.now); got != tc.want {
			t.Errorf("%s: heartbeatStale(%v) = %v, want %v", tc.name, tc.now, got, tc.want)
		}
	}
}

// A fresh conn must start with zero heartbeat state so the watchdog cannot fire
// on its first tick. This replaces the old resetHeartbeat test: where a reused
// Session needed an explicit reset to avoid a reconnect loop, a per-Open conn
// gets zero-valued lastHB/lastAck for free from newConn.
func TestFreshConnHasZeroHeartbeatState(t *testing.T) {
	s := &Session{log: slogtest.Make(t, nil)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := s.newConn(ctx, cancel)
	c.interval = 40 * time.Second
	if !c.lastHB.IsZero() || !c.lastAck.IsZero() {
		t.Fatalf("fresh conn carried heartbeat state: lastHB=%v lastAck=%v", c.lastHB, c.lastAck)
	}
	if c.heartbeatStale(time.Now()) {
		t.Fatal("heartbeatStale fired on a fresh connection; would cause a reconnect loop")
	}
}

// The fix must not break the happy path: with a receiver draining prioch and a
// live context, writeHeartbeat still delivers the heartbeat op.
func TestWriteHeartbeatDeliversWhenDrained(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Session{log: slogtest.Make(t, nil)}
	c := &conn{
		s:      s,
		ctx:    ctx,
		cancel: cancel,
		prioch: make(chan *Op),
	}

	go c.writeHeartbeat()

	select {
	case op := <-c.prioch:
		if op.Op != 1 {
			t.Fatalf("heartbeat op = %d, want 1", op.Op)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("writeHeartbeat did not deliver to a draining receiver")
	}
}
