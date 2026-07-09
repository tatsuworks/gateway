package statepsql

import (
	"context"
	"os"
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
