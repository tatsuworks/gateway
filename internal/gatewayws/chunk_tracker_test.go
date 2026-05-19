package gatewayws

import (
	"testing"
	"time"
)

func TestChunkTracker_DrainCompletesAfterAllChunks(t *testing.T) {
	tr := newChunkTracker()
	tr.registerRequest(123)

	if tr.pendingCount() != 1 {
		t.Fatalf("pending: got %d want 1", tr.pendingCount())
	}

	tr.recordChunk(123, 0, 3)
	tr.recordChunk(123, 1, 3)
	if tr.pendingCount() != 1 {
		t.Fatalf("should still be pending after 2/3 chunks")
	}

	tr.recordChunk(123, 2, 3)
	select {
	case <-tr.drained():
	case <-time.After(50 * time.Millisecond):
		t.Fatalf("drained channel did not fire after all chunks arrived")
	}
	if tr.pendingCount() != 0 {
		t.Fatalf("pending: got %d want 0", tr.pendingCount())
	}
}

func TestChunkTracker_DrainedFiresImmediatelyWhenEmpty(t *testing.T) {
	tr := newChunkTracker()
	select {
	case <-tr.drained():
	case <-time.After(50 * time.Millisecond):
		t.Fatalf("empty tracker should report drained immediately")
	}
}

func TestChunkTracker_MultipleGuilds(t *testing.T) {
	tr := newChunkTracker()
	tr.registerRequest(1)
	tr.registerRequest(2)
	tr.recordChunk(1, 0, 1)
	if tr.pendingCount() != 1 {
		t.Fatalf("guild 1 drained, guild 2 still pending; got %d", tr.pendingCount())
	}
	tr.recordChunk(2, 0, 2)
	tr.recordChunk(2, 1, 2)
	if tr.pendingCount() != 0 {
		t.Fatalf("both guilds should be drained; got %d", tr.pendingCount())
	}
}

func TestChunkTracker_UnregisteredChunkIgnored(t *testing.T) {
	tr := newChunkTracker()
	tr.recordChunk(999, 0, 1)
	if tr.pendingCount() != 0 {
		t.Fatalf("unregistered guild should not create pending entry")
	}
}
