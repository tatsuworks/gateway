package gatewayws

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"cdr.dev/slog/sloggers/slogtest"
)

// After the conn refactor, two sequential connections must not share send
// channels: a fresh conn allocates its own wch/prioch, so a producer for
// connection B can never reach connection A's writer (the captured-locals
// orphaning bug).
func TestSequentialConnsDoNotShareChannels(t *testing.T) {
	s := &Session{log: slogtest.Make(t, nil)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := s.newConn(ctx, cancel)
	b := s.newConn(ctx, cancel)
	if a.wch == b.wch || a.prioch == b.prioch {
		t.Fatal("two connections share send channels; reconnect can orphan the writer")
	}
}

// ForceIdentify with no active connection must still take effect on the next
// Open: the atomic flag persists even though Cancel is a no-op.
func TestForceIdentifyWithNilCurStillFlags(t *testing.T) {
	s := &Session{log: slogtest.Make(t, nil)}
	s.ForceIdentify() // cur is nil
	if atomic.LoadInt32(&s.forceIdentify) != 1 {
		t.Fatal("ForceIdentify did not set the flag when no connection was active")
	}
}

// RequestGuildMembers is dropped (not buffered) when no connection is active.
// It must return promptly without panicking on a nil cur.
func TestRequestGuildMembersDroppedWhenDisconnected(t *testing.T) {
	s := &Session{log: slogtest.Make(t, nil)}
	done := make(chan struct{})
	go func() { s.RequestGuildMembers(123); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RequestGuildMembers blocked on a nil cur")
	}
}

// Status/LongLastAck report disconnected when there is no active connection.
func TestStatusWhenDisconnected(t *testing.T) {
	s := &Session{log: slogtest.Make(t, nil), shardID: 9}
	if got := s.Status(); got == "" {
		t.Fatal("Status returned empty on nil cur")
	}
	if !s.LongLastAck(time.Minute) {
		t.Fatal("LongLastAck should report true (no acks) on nil cur")
	}
}

// Status/LongLastAck run on the manager goroutine while the read-loop and
// heartbeat goroutines write curState/lastAck/lastHB. Those reads and writes
// must go through the conn mutex; -race fails here otherwise.
func TestStatusRaceWithConnectionWrites(t *testing.T) {
	s := &Session{log: slogtest.Make(t, nil), shardID: 7}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := s.newConn(ctx, cancel)
	s.cur.Store(c)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			c.setState("handle internal event X")
			c.markAck(time.Now())
		}
		close(done)
	}()
	for i := 0; i < 1000; i++ {
		_ = s.Status()
		_ = s.LongLastAck(time.Minute)
	}
	<-done
}
