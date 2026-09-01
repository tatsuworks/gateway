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
