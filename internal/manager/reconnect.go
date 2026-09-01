package manager

import (
	"context"
	"math/rand"
	"time"
)

const (
	// reconnectBackoffBase is the wait after a connection that was healthy (see
	// reconnectHealthyUptime) drops — the historical flat retry interval, kept so
	// an ordinary reconnect is still fast.
	reconnectBackoffBase = time.Second
	// reconnectBackoffMax caps the wait. A shard that genuinely cannot connect
	// settles at roughly one attempt per minute instead of one per second: the
	// flat 1s retry produced ~3,300 "websocket closed" lines in 10 minutes from a
	// single pod, which rotated the log buffer fast enough to destroy the
	// evidence of when the failure started.
	reconnectBackoffMax = time.Minute
	// reconnectJitterDivisor sets the jitter band: up to delay/divisor is added
	// to each wait so a fleet of 1024 shards knocked offline together does not
	// retry in lockstep.
	reconnectJitterDivisor = 4
	// reconnectHealthyUptime is how long a connection must last to count as
	// healthy. Anything shorter is treated as a failed attempt and escalates the
	// backoff; anything longer resets it, so a shard that ran for hours before a
	// single drop reconnects immediately rather than inheriting a stale delay.
	reconnectHealthyUptime = time.Minute
)

// reconnectDelay returns how long to wait before the next connect attempt given
// the number of consecutive failed attempts so far (0 = the connection was
// healthy). It doubles from reconnectBackoffBase up to reconnectBackoffMax and
// adds jitter.
func reconnectDelay(failures int) time.Duration {
	d := reconnectBackoffMax
	switch {
	case failures <= 0:
		d = reconnectBackoffBase
	case failures < 32:
		// Bounded shift: base<<31 stays well inside int64, and anything that
		// reaches the cap below stops mattering.
		if shifted := reconnectBackoffBase << uint(failures); shifted < reconnectBackoffMax {
			d = shifted
		}
	}

	return d + time.Duration(rand.Int63n(int64(d)/reconnectJitterDivisor+1))
}

// nextFailureCount folds one connection's lifetime into the consecutive-failure
// count driving reconnectDelay.
func nextFailureCount(failures int, uptime time.Duration) int {
	if uptime >= reconnectHealthyUptime {
		return 0
	}
	return failures + 1
}

// sleepCtx waits for d, returning false if ctx is cancelled first so the caller
// can stop instead of holding shutdown open for a full backoff interval.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
