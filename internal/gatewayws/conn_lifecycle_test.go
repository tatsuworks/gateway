package gatewayws

import "testing"

// After the conn refactor, two sequential connections must not share send
// channels: a fresh conn allocates its own wch/prioch, so a producer for
// connection B can never reach connection A's writer (the captured-locals
// orphaning bug). Re-enabled with a real body in Task 4 once conn exists.
func TestSequentialConnsDoNotShareChannels(t *testing.T) {
	t.Skip("enabled in Task 3 after conn type exists")
}
