package gatewayws

import (
	"testing"
	"time"
)

// The reconnect ladder resets on a connection that stayed up long enough to
// count as healthy. It used to be handed the total duration of Session.Open,
// which wraps etcd setup and acquireIdentifyLock — and that lock waits up to
// 160s. An attempt could therefore sit on the lock for well over a minute, fail
// the very first dial, and still be scored as a healthy connection, resetting
// the backoff to its base every time. That is exactly the flat 1s retry the
// ladder exists to remove, in exactly the fleet-wide contention it exists for.
// Open now reports connected time, measured from the READY/RESUMED milestone.

// An attempt that never authenticated was never connected, however long it
// spent getting there.
func TestReadyForIsZeroWhenTheConnectionNeverAuthenticated(t *testing.T) {
	db := &shardInfoRecorder{}
	_, c := newResumableSession(t, db)

	// Stands in for a long acquireIdentifyLock wait followed by a failed dial.
	if got := c.readyFor(time.Now().Add(3 * time.Minute)); got != 0 {
		t.Fatalf("readyFor() = %v for a connection that never reached READY, want 0", got)
	}
}

// Once authenticated, connected time runs from that milestone — not from the
// start of the attempt, which includes the pre-connect work.
func TestReadyForMeasuresFromTheReadyMilestone(t *testing.T) {
	db := &shardInfoRecorder{}
	_, c := newResumableSession(t, db)

	ready := time.Now()
	c.markReady(ready)

	if got := c.readyFor(ready.Add(90 * time.Second)); got != 90*time.Second {
		t.Fatalf("readyFor() = %v, want 90s measured from the ready milestone", got)
	}
}

// Connected uptime must stop at the disconnect, not at run's return. run defers
// persistShardInfo, which writes to Postgres on context.Background() with NO
// deadline, so teardown can outlast the connection by an unbounded amount. If
// that stall counted as uptime, a connection that lasted seconds would clear
// reconnectHealthyUptime and reset the manager's backoff ladder — during a
// database stall, which is exactly when it should be escalating — and the
// curated `connected_for` incident log line would report the stall as uptime.
func TestConnectedUptimeExcludesTeardown(t *testing.T) {
	db := &shardInfoRecorder{}
	_, c := newResumableSession(t, db)

	ready := time.Now()
	c.markReady(ready)
	// The websocket read loop fails five seconds in: an unhealthy connection.
	c.markDisconnected(ready.Add(5 * time.Second))

	// Teardown then stalls for minutes on the unbounded shard-info write.
	if got := c.readyFor(ready.Add(10 * time.Minute)); got != 5*time.Second {
		t.Fatalf("readyFor() = %v, want 5s — teardown time is not connected uptime", got)
	}
}

// A connection still running has no disconnect stamp, so uptime is measured to
// the caller's clock as before.
func TestConnectedUptimeRunsToNowWhileStillConnected(t *testing.T) {
	db := &shardInfoRecorder{}
	_, c := newResumableSession(t, db)

	ready := time.Now()
	c.markReady(ready)

	if got := c.readyFor(ready.Add(90 * time.Second)); got != 90*time.Second {
		t.Fatalf("readyFor() = %v, want 90s for a live connection", got)
	}
}

// A connection that dropped before ever authenticating reports 0 however long
// its teardown took, so the ladder escalates rather than resetting.
func TestNeverAuthenticatedReportsZeroDespiteDisconnectStamp(t *testing.T) {
	db := &shardInfoRecorder{}
	_, c := newResumableSession(t, db)

	c.markDisconnected(time.Now())

	if got := c.readyFor(time.Now().Add(3 * time.Minute)); got != 0 {
		t.Fatalf("readyFor() = %v for a connection that never reached READY, want 0", got)
	}
}
