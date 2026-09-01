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
func TestSleepCtxReturnsEarlyOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	if sleepCtx(ctx, time.Hour) {
		t.Fatal("sleepCtx returned true for a cancelled context; want false")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("sleepCtx took %v to notice cancellation; want immediate", elapsed)
	}
}

// A completed wait reports true so the caller keeps looping.
func TestSleepCtxReportsCompletion(t *testing.T) {
	if !sleepCtx(context.Background(), time.Millisecond) {
		t.Fatal("sleepCtx returned false after the delay elapsed; want true")
	}
}

// A connection that stayed up long enough to be doing real work resets the
// backoff, so an hours-long healthy session followed by one drop reconnects
// promptly instead of inheriting a stale minute-long delay.
func TestNextFailureCountResetsAfterHealthyUptime(t *testing.T) {
	if got := nextFailureCount(5, reconnectHealthyUptime); got != 0 {
		t.Fatalf("nextFailureCount(5, healthy) = %d, want 0", got)
	}
	if got := nextFailureCount(5, reconnectHealthyUptime+time.Hour); got != 0 {
		t.Fatalf("nextFailureCount(5, long uptime) = %d, want 0", got)
	}
	if got := nextFailureCount(5, time.Millisecond); got != 6 {
		t.Fatalf("nextFailureCount(5, instant failure) = %d, want 6", got)
	}
	if got := nextFailureCount(0, time.Millisecond); got != 1 {
		t.Fatalf("nextFailureCount(0, instant failure) = %d, want 1", got)
	}
}
