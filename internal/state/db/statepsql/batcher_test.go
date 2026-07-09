package statepsql

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"cdr.dev/slog/sloggers/sloghuman"
)

type testEvent struct {
	route, dedup uint64
	tag          int
}

func (e testEvent) RouteKey() uint64 { return e.route }
func (e testEvent) DedupKey() any    { return e.dedup }

func newTestBatcher(t *testing.T, shards, maxBatch int, flushInterval time.Duration) (*ShardedBatcher[testEvent], <-chan []testEvent, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	batches := make(chan []testEvent, 32)
	process := func(_ context.Context, events []testEvent) error {
		batches <- append([]testEvent(nil), events...)
		return nil
	}
	b := NewShardedBatcher(ctx, shards, maxBatch, flushInterval, process, sloghuman.Make(os.Stderr))
	return b, batches, cancel
}

func TestShardedBatcherDedupesByKey(t *testing.T) {
	b, batches, cancel := newTestBatcher(t, 1, 2, 10*time.Second)
	defer cancel()

	ctx := context.Background()
	// Two events share dedup=42 (later tag should win). Third event with
	// dedup=99 makes the unique count hit maxBatch=2, triggering flush.
	_ = b.Send(ctx, testEvent{route: 1, dedup: 42, tag: 1})
	_ = b.Send(ctx, testEvent{route: 1, dedup: 42, tag: 2})
	_ = b.Send(ctx, testEvent{route: 1, dedup: 99, tag: 3})

	select {
	case batch := <-batches:
		if len(batch) != 2 {
			t.Fatalf("expected 2 events after dedup, got %d: %+v", len(batch), batch)
		}
		seen := map[uint64]int{}
		for _, ev := range batch {
			seen[ev.dedup] = ev.tag
		}
		if seen[42] != 2 {
			t.Errorf("expected dedup=42 to keep later tag=2, got %d", seen[42])
		}
		if seen[99] != 3 {
			t.Errorf("expected dedup=99 tag=3, got %d", seen[99])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for flush")
	}
}

func TestShardedBatcherFlushesAtMaxSize(t *testing.T) {
	b, batches, cancel := newTestBatcher(t, 1, 3, 10*time.Second)
	defer cancel()

	ctx := context.Background()
	for i := range 3 {
		_ = b.Send(ctx, testEvent{route: 1, dedup: uint64(i)})
	}

	select {
	case batch := <-batches:
		if len(batch) != 3 {
			t.Fatalf("expected batch of 3 at maxBatchSize, got %d", len(batch))
		}
	case <-time.After(time.Second):
		t.Fatal("max-size flush did not trigger")
	}
}

func TestShardedBatcherFlushesOnInterval(t *testing.T) {
	b, batches, cancel := newTestBatcher(t, 1, 1000, 20*time.Millisecond)
	defer cancel()

	ctx := context.Background()
	_ = b.Send(ctx, testEvent{route: 1, dedup: 1})
	_ = b.Send(ctx, testEvent{route: 1, dedup: 2})

	select {
	case batch := <-batches:
		if len(batch) != 2 {
			t.Fatalf("expected interval flush of 2 events, got %d", len(batch))
		}
	case <-time.After(time.Second):
		t.Fatal("interval flush did not trigger")
	}
}

func TestShardedBatcherFlushForShardIsSynchronous(t *testing.T) {
	// A long flush interval means nothing flushes on its own within the test
	// window; FlushForShard must force the queued events out and only return
	// once process() has run.
	b, batches, cancel := newTestBatcher(t, 4, 1000, 10*time.Second)
	defer cancel()

	ctx := context.Background()
	_ = b.Send(ctx, testEvent{route: 7, dedup: 1})
	_ = b.Send(ctx, testEvent{route: 7, dedup: 2})

	if err := b.FlushForShard(ctx, 7); err != nil {
		t.Fatalf("FlushForShard: %v", err)
	}

	// On return the batch must already be available (not pending the ticker).
	select {
	case batch := <-batches:
		if len(batch) != 2 {
			t.Fatalf("expected 2 flushed events, got %d: %+v", len(batch), batch)
		}
	default:
		t.Fatal("FlushForShard returned before the batch was processed")
	}
}

func TestShardedBatcherFlushForShardSurfacesAsyncError(t *testing.T) {
	ctx := t.Context()

	wantErr := errors.New("batch write failed")
	var calls atomic.Int64
	flushed := make(chan struct{}, 1)
	process := func(_ context.Context, _ []testEvent) error {
		if calls.Add(1) == 1 {
			flushed <- struct{}{} // the size-triggered async flush ran...
			return wantErr        // ...and failed transiently
		}
		return nil
	}
	b := NewShardedBatcher(ctx, 1, 2, 10*time.Second, process, sloghuman.Make(os.Stderr), WithFlushErrorTracking())

	// Two events hit maxBatchSize and trigger an async flush whose error the
	// worker logs and drops. FlushForShard must still surface it — otherwise
	// CompleteGuildBackfill would stamp the guild complete over lost members.
	_ = b.Send(ctx, testEvent{route: 1, dedup: 1})
	_ = b.Send(ctx, testEvent{route: 1, dedup: 2})

	// Wait until that size-triggered flush has actually run before flushing, so
	// the batch is already drained and FlushForShard can only report the error
	// via the async latch — not by re-flushing the events synchronously itself.
	// Without this the flushReq could win the select race and pass even if the
	// latch were broken.
	select {
	case <-flushed:
	case <-time.After(time.Second):
		t.Fatal("size-triggered async flush never ran")
	}

	if err := b.FlushForShard(ctx, 1); err == nil {
		t.Fatal("FlushForShard swallowed the dropped async flush error")
	}

	// The recorded error is cleared on read: a later successful flush is clean.
	_ = b.Send(ctx, testEvent{route: 1, dedup: 3})
	if err := b.FlushForShard(ctx, 1); err != nil {
		t.Fatalf("FlushForShard should be clean after the error was consumed, got %v", err)
	}
}

func TestShardedBatcherFlushForShardErrorIsPerRouteKey(t *testing.T) {
	ctx := t.Context()

	wantErr := errors.New("batch write failed")
	var calls atomic.Int64
	flushed := make(chan struct{}, 1)
	process := func(_ context.Context, _ []testEvent) error {
		if calls.Add(1) == 1 {
			flushed <- struct{}{} // route-1's size-triggered flush ran...
			return wantErr        // ...and failed transiently
		}
		return nil
	}
	// Single worker so routes 1 and 2 share it (as ~all guilds share one of
	// maxConns=4 workers in prod). maxBatchSize 2 so two route-1 events flush.
	b := NewShardedBatcher(ctx, 1, 2, 10*time.Second, process, sloghuman.Make(os.Stderr), WithFlushErrorTracking())

	_ = b.Send(ctx, testEvent{route: 1, dedup: 1})
	_ = b.Send(ctx, testEvent{route: 1, dedup: 2})
	select {
	case <-flushed:
	case <-time.After(time.Second):
		t.Fatal("size-triggered async flush never ran")
	}

	// Route 2 shares the worker but was not in the failed batch. It must NOT
	// consume route 1's dropped-flush error — otherwise route 1 would later
	// flush clean and stamp its backfill complete over its missing members.
	if err := b.FlushForShard(ctx, 2); err != nil {
		t.Fatalf("route 2 must not see route 1's dropped async error, got %v", err)
	}

	// Route 1 must still surface its own dropped-flush error.
	if err := b.FlushForShard(ctx, 1); err == nil {
		t.Fatal("route 1's FlushForShard must surface its own dropped async error")
	}
}

func TestShardedBatcherFlushErrorTrackingOffByDefault(t *testing.T) {
	ctx := t.Context()

	wantErr := errors.New("batch write failed")
	var calls atomic.Int64
	flushed := make(chan struct{}, 1)
	process := func(_ context.Context, _ []testEvent) error {
		if calls.Add(1) == 1 {
			flushed <- struct{}{}
			return wantErr
		}
		return nil
	}
	// No WithFlushErrorTracking (the presence/guild batchers, which never call
	// FlushForShard): a dropped async flush must not be retained, else entries
	// would accumulate for the life of the process with nothing to consume them.
	b := NewShardedBatcher(ctx, 1, 2, 10*time.Second, process, sloghuman.Make(os.Stderr))

	_ = b.Send(ctx, testEvent{route: 1, dedup: 1})
	_ = b.Send(ctx, testEvent{route: 1, dedup: 2})
	select {
	case <-flushed:
	case <-time.After(time.Second):
		t.Fatal("size-triggered async flush never ran")
	}

	if err := b.FlushForShard(ctx, 1); err != nil {
		t.Fatalf("without tracking, FlushForShard must not retain the dropped error, got %v", err)
	}
}

func TestShardedBatcherFlushForShardEmptyIsNoop(t *testing.T) {
	b, _, cancel := newTestBatcher(t, 2, 1000, 10*time.Second)
	defer cancel()

	if err := b.FlushForShard(context.Background(), 3); err != nil {
		t.Fatalf("FlushForShard on empty shard should be a no-op, got %v", err)
	}
}

func TestShardedBatcherRoutesByKey(t *testing.T) {
	// With 4 shards, RouteKey 0 and RouteKey 2 land on different shards
	// (0%4=0, 2%4=2). Each shard has its own goroutine and batch, so events
	// from different routes must never appear in the same batch.
	b, batches, cancel := newTestBatcher(t, 4, 1000, 20*time.Millisecond)
	defer cancel()

	ctx := context.Background()
	for i := range 5 {
		_ = b.Send(ctx, testEvent{route: 0, dedup: uint64(i)})
		_ = b.Send(ctx, testEvent{route: 2, dedup: uint64(100 + i)})
	}

	deadline := time.After(time.Second)
	seenRoutes := map[uint64]int{}
	for seenRoutes[0]+seenRoutes[2] < 10 {
		select {
		case batch := <-batches:
			if len(batch) == 0 {
				continue
			}
			route := batch[0].route
			for _, ev := range batch {
				if ev.route != route {
					t.Fatalf("batch mixed routes: found %d and %d", route, ev.route)
				}
			}
			seenRoutes[route] += len(batch)
		case <-deadline:
			t.Fatalf("timed out; saw routes: %+v", seenRoutes)
		}
	}
	if seenRoutes[0] != 5 || seenRoutes[2] != 5 {
		t.Errorf("expected 5 events per route, got %+v", seenRoutes)
	}
}
