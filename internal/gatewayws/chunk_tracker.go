package gatewayws

import "sync"

type chunkEntry struct {
	received int32
	expected int32
}

type chunkTracker struct {
	mu        sync.Mutex
	pending   map[int64]*chunkEntry
	drainedCh chan struct{}
}

func newChunkTracker() *chunkTracker {
	t := &chunkTracker{
		pending:   make(map[int64]*chunkEntry),
		drainedCh: make(chan struct{}, 1),
	}
	t.signalIfEmptyLocked()
	return t
}

func (t *chunkTracker) registerRequest(guildID int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.pending[guildID]; ok {
		return
	}
	t.pending[guildID] = &chunkEntry{}
	select {
	case <-t.drainedCh:
	default:
	}
}

func (t *chunkTracker) recordChunk(guildID int64, _, count int32) {
	if count <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.pending[guildID]
	if !ok {
		return
	}
	e.received++
	if count > e.expected {
		e.expected = count
	}
	if e.received >= e.expected {
		delete(t.pending, guildID)
		t.signalIfEmptyLocked()
	}
}

func (t *chunkTracker) drained() <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.signalIfEmptyLocked()
	return t.drainedCh
}

func (t *chunkTracker) pendingCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.pending)
}

func (t *chunkTracker) signalIfEmptyLocked() {
	if len(t.pending) == 0 {
		select {
		case t.drainedCh <- struct{}{}:
		default:
		}
	}
}
