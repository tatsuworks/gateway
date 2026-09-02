package manager

import (
	"context"
	"testing"
	"time"
)

// wakeShard's token is buffered and only waitBeforeReconnect drains it. A
// RestartShard landing in the window after the wait returns but before Open
// republishes Session.cur finds Cancel a no-op AND leaves a token behind. If the
// connection that follows is healthy and runs for hours, the token outlives it:
// when that connection eventually drops for an unrelated reason, the next
// backoff is skipped, the failure ladder resets, and the log claims an operator
// asked for a reconnect that nobody requested.
//
// Draining immediately before the connect attempt confines a token to the
// interval it was meant for.
func TestDrainWakeDiscardsATokenBufferedOutsideTheWait(t *testing.T) {
	wake := make(chan struct{}, 1)
	wake <- struct{}{}

	drainWake(wake)

	start := time.Now()
	ok, woken := waitBeforeReconnect(context.Background(), wake, 20*time.Millisecond)
	if !ok {
		t.Fatal("wait reported cancellation on a live context")
	}
	if woken {
		t.Fatal("a stale token cut short a later backoff and would mis-log it as a management request")
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("wait returned after %v, want the full delay", elapsed)
	}
}

// The drain runs on every connect attempt, so it must never block: not on an
// empty channel, and not on the nil channel an unknown shard would present.
func TestDrainWakeNeverBlocks(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		drainWake(make(chan struct{}, 1))
		drainWake(nil)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("drainWake blocked; it runs on every connect attempt")
	}
}

// A token sent while the loop IS waiting must still cut the wait short — the
// drain must not undo the wake path it sits next to.
func TestDrainWakeDoesNotDefeatARealWake(t *testing.T) {
	wake := make(chan struct{}, 1)
	drainWake(wake)

	go func() {
		time.Sleep(10 * time.Millisecond)
		wake <- struct{}{}
	}()

	ok, woken := waitBeforeReconnect(context.Background(), wake, time.Minute)
	if !ok {
		t.Fatal("wait reported cancellation on a live context")
	}
	if !woken {
		t.Fatal("a wake sent during the wait did not interrupt it")
	}
}
