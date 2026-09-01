package manager

import (
	"context"
	"testing"
	"time"
)

// The reconnect loop used to sleep a flat 1s between attempts. Against a shard
// stuck dialing a dead host that is ~3,300 error lines in 10 minutes on a
// single pod — enough to rotate the log buffer and destroy the evidence of when
// the loop started. Back off instead.

// reconnectDelay grows exponentially from the base and is capped, so a shard
// that cannot connect settles at one attempt per minute instead of one per
// second. Jitter keeps 1024 shards from retrying in lockstep, so the delay is
// asserted as a range rather than an exact value.
func TestReconnectDelayBacksOffExponentiallyAndCaps(t *testing.T) {
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{0, reconnectBackoffBase},
		{1, 2 * reconnectBackoffBase},
		{2, 4 * reconnectBackoffBase},
		{3, 8 * reconnectBackoffBase},
		{6, reconnectBackoffMax},
		{60, reconnectBackoffMax},
		{1000, reconnectBackoffMax},
	}
	for _, tc := range cases {
		want := tc.want
		if want > reconnectBackoffMax {
			want = reconnectBackoffMax
		}
		maxWithJitter := want + want/reconnectJitterDivisor
		for i := 0; i < 50; i++ {
			got := reconnectDelay(tc.failures)
			if got < want || got > maxWithJitter {
				t.Fatalf("reconnectDelay(%d) = %v, want within [%v, %v]",
					tc.failures, got, want, maxWithJitter)
			}
		}
	}
}

// A huge failure count must not overflow the shift into a negative or tiny
// duration — the cap has to hold for any input.
func TestReconnectDelayNeverExceedsCapOnOverflow(t *testing.T) {
	for _, failures := range []int{62, 63, 64, 65, 1 << 20} {
		got := reconnectDelay(failures)
		ceiling := reconnectBackoffMax + reconnectBackoffMax/reconnectJitterDivisor
		if got < reconnectBackoffMax || got > ceiling {
			t.Fatalf("reconnectDelay(%d) = %v, want within [%v, %v]",
				failures, got, reconnectBackoffMax, ceiling)
		}
	}
}

// Jitter must actually vary, otherwise the whole fleet retries in lockstep.
func TestReconnectDelayIsJittered(t *testing.T) {
	seen := map[time.Duration]struct{}{}
	for i := 0; i < 200; i++ {
		seen[reconnectDelay(3)] = struct{}{}
	}
	if len(seen) < 2 {
		t.Fatalf("reconnectDelay(3) produced %d distinct values over 200 calls; want jitter", len(seen))
	}
}

// The backoff wait must be abortable: on shutdown the loop has to return
// immediately rather than hold the process open for up to a minute.
func TestWaitBeforeReconnectReturnsEarlyOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	ok, woken := waitBeforeReconnect(ctx, nil, time.Hour)
	if ok {
		t.Fatal("waitBeforeReconnect reported ok for a cancelled context; want false")
	}
	if woken {
		t.Fatal("waitBeforeReconnect reported woken for a cancelled context")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("waitBeforeReconnect took %v to notice cancellation; want immediate", elapsed)
	}
}

// A completed wait reports ok (keep looping) and not woken (keep the ladder).
func TestWaitBeforeReconnectReportsCompletion(t *testing.T) {
	ok, woken := waitBeforeReconnect(context.Background(), nil, time.Millisecond)
	if !ok {
		t.Fatal("waitBeforeReconnect reported not-ok after the delay elapsed; want ok")
	}
	if woken {
		t.Fatal("waitBeforeReconnect reported woken with no wake signal")
	}
}

// The regression this guards: Open clears Session.cur on return, so for the
// whole reconnect wait Session.Cancel is a no-op and RestartShard reports
// success while doing nothing. With a flat 1s retry that dead window was ~1s;
// under backoff it would be up to ~75s -- squarely on top of gwForceIdentify,
// the operator workaround for the very incident this backoff was added for. An
// explicit management request must cut the wait short.
func TestWaitBeforeReconnectWokenByManagementRequest(t *testing.T) {
	wake := make(chan struct{}, 1)
	wake <- struct{}{}

	start := time.Now()
	ok, woken := waitBeforeReconnect(context.Background(), wake, time.Hour)
	if !ok {
		t.Fatal("waitBeforeReconnect reported not-ok after a wake; want ok (keep looping)")
	}
	if !woken {
		t.Fatal("waitBeforeReconnect did not report woken; the caller must restart the failure ladder")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("waitBeforeReconnect waited %v despite a pending wake; want immediate", elapsed)
	}
}

// wakeShard is what RestartShard/ForceIdentify call. It must be non-blocking
// (the gRPC handler must never stall on a shard that is not waiting), must
// coalesce, and must hold a signal sent while nobody is waiting so an explicit
// restart is never silently dropped.
func TestWakeShardIsNonBlockingAndCoalesces(t *testing.T) {
	m := &Manager{wake: map[int]chan struct{}{7: make(chan struct{}, 1)}}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			m.wakeShard(7)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("wakeShard blocked; it must never stall the gRPC handler")
	}

	// The signal was held even though nobody was waiting, and 100 sends
	// coalesced into one pending token.
	if _, woken := waitBeforeReconnect(context.Background(), m.wake[7], time.Hour); !woken {
		t.Fatal("a wake sent while nobody was waiting was dropped")
	}
	if _, woken := waitBeforeReconnect(context.Background(), m.wake[7], time.Millisecond); woken {
		t.Fatal("wake signals did not coalesce; a second token was left pending")
	}
}

// A restart aimed at a shard with no reconnect loop registered must be a no-op,
// not a panic (nil map entry).
func TestWakeShardUnknownShardIsNoop(t *testing.T) {
	m := &Manager{wake: map[int]chan struct{}{}}
	m.wakeShard(1234)
}

// A connection that stayed up long enough to be doing real work resets the
// backoff, so an hours-long healthy session followed by one drop reconnects
// promptly instead of inheriting a stale minute-long delay.
func TestNextFailureCountResetsAfterHealthyUptime(t *testing.T) {
	if got := nextFailureCount(5, reconnectHealthyUptime, false); got != 0 {
		t.Fatalf("nextFailureCount(5, healthy) = %d, want 0", got)
	}
	if got := nextFailureCount(5, reconnectHealthyUptime+time.Hour, false); got != 0 {
		t.Fatalf("nextFailureCount(5, long uptime) = %d, want 0", got)
	}
	if got := nextFailureCount(5, time.Millisecond, false); got != 6 {
		t.Fatalf("nextFailureCount(5, instant failure) = %d, want 6", got)
	}
	if got := nextFailureCount(0, time.Millisecond, false); got != 1 {
		t.Fatalf("nextFailureCount(0, instant failure) = %d, want 1", got)
	}
}

// Discarding the resume tuple repoints the next dial at the main gateway URL.
// The failures were earned against a different host, so the ladder restarts --
// otherwise the escape waits out a delay that predicts nothing. Measured on
// staging: 9.0s of a 15.8s escape was exactly this dead wait.
func TestNextFailureCountResetsWhenResumeDiscarded(t *testing.T) {
	if got := nextFailureCount(3, time.Millisecond, true); got != 0 {
		t.Fatalf("nextFailureCount(3, instant failure, discarded) = %d, want 0", got)
	}
	if got := nextFailureCount(31, time.Millisecond, true); got != 0 {
		t.Fatalf("nextFailureCount(31, instant failure, discarded) = %d, want 0", got)
	}
	// And the reset actually buys back the time: base delay, not an escalated one.
	if got := reconnectDelay(nextFailureCount(3, time.Millisecond, true)); got >= 2*reconnectBackoffBase {
		t.Fatalf("delay after a discard = %v, want ~%v (the base rung)", got, reconnectBackoffBase)
	}
}
