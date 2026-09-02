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
	// reconnectHealthyUptime is how long a connection must stay authenticated to
	// count as healthy. Anything shorter is treated as a failed attempt and
	// escalates the backoff; anything longer resets it, so a shard that ran for
	// hours before a single drop reconnects immediately rather than inheriting a
	// stale delay. Measured from the READY/RESUMED milestone, not from the start
	// of the connect attempt -- see nextFailureCount.
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

// nextFailureCount folds one connection's outcome into the consecutive-failure
// count driving reconnectDelay.
//
// connected is time spent authenticated (see Session.Open), never the duration
// of the attempt as a whole. Open covers etcd setup and an identify-lock wait of
// up to 160s before any websocket exists, so an attempt that waits out the lock
// and then fails its first dial takes minutes while connecting for none of it;
// scoring that as healthy reset the ladder to base on every attempt, in exactly
// the fleet-wide identify contention it is meant to damp.
//
// resumeDiscarded restarts the ladder because the attempt threw away the resume
// tuple: the next dial goes to the main gateway URL, a different host from the
// one these failures came from, so the delay they earned says nothing about it.
// Without the reset the escape from a dead resume host pays for a wait it no
// longer needs -- 9.0s of a 15.8s recovery, measured on staging.
func nextFailureCount(failures int, connected time.Duration, resumeDiscarded bool) int {
	if resumeDiscarded || connected >= reconnectHealthyUptime {
		return 0
	}
	return failures + 1
}

// drainWake discards a wake token that was buffered outside any wait, so a
// signal only ever cuts short the interval it was meant for.
//
// wakeShard's channel is depth-1 and buffered on purpose: a signal sent while
// the loop is not waiting is held rather than lost. That is right for the window
// between waits, but it also means a RestartShard landing after a wait returns
// and before Open republishes Session.cur leaves a token behind that nothing
// consumed — Cancel was a no-op there too. If the connection that follows is
// healthy and runs for hours, the token outlives it: the next unrelated drop
// skips its backoff, resets the failure ladder, and logs a management request
// nobody made. Draining immediately before the connect attempt confines the
// token to the wait it was sent for.
//
// Non-blocking, and a no-op on the nil channel an unknown shard presents, since
// it runs on every connect attempt.
func drainWake(wake <-chan struct{}) {
	select {
	case <-wake:
	default:
	}
}

// waitBeforeReconnect waits out the backoff before the next connect attempt.
//
// ok is false only when ctx was cancelled, so the caller stops instead of
// holding shutdown open for a full backoff interval. woken reports that an
// explicit management request (see wakeShard) cut the wait short; the caller
// restarts the failure ladder in that case, because an operator asking for a
// reconnect should get the prompt first attempt, not the escalated delay.
//
// The wake path matters because Open clears Session.cur on return: for the
// whole of this wait Session.Cancel is a no-op, so without it RestartShard
// would report success and do nothing until the delay expired.
func waitBeforeReconnect(ctx context.Context, wake <-chan struct{}, d time.Duration) (ok, woken bool) {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return false, false
	case <-wake:
		return true, true
	case <-t.C:
		return true, false
	}
}
