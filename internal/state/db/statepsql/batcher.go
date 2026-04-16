package statepsql

import (
	"context"
	"time"

	"cdr.dev/slog"
)

type MemberEvent struct {
	GuildID int64
	UserID  int64
	Raw     []byte
	IsNew   bool
}
type PresenceEvent struct {
	GuildID int64
	UserID  int64
	Raw     []byte
}
type GuildEvent struct {
	GuildID int64
	Raw     []byte
}

// ShardedBatcher fans events across N independent batch workers, routed by
// routeKey(ev) % N. Because Discord maps each guild to exactly one shard,
// routing by guildID guarantees that events for the same rows always land on
// the same worker — two concurrent flushes can never contend on the same row.
//
// Deduplication within a batch uses a separate dedupKey so that, e.g.,
// different members in the same guild are kept as distinct entries while
// duplicate updates to the same member collapse.
type ShardedBatcher[T any] struct {
	chans    []chan T
	routeKey func(T) uint64
}

func (s *ShardedBatcher[T]) Send(ctx context.Context, ev T) error {
	ch := s.chans[s.routeKey(ev)%uint64(len(s.chans))]
	select {
	case ch <- ev:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NewShardedBatcher spawns `shards` batch workers, each with its own channel
// and in-flight flush slot. routeKey determines which worker an event goes to;
// dedupKey determines which events overwrite each other within a batch.
//
// Flushes happen when a worker's batch reaches maxBatchSize or every
// flushInterval. While a flush is in flight, the worker keeps draining its
// channel into the next batch; a second flush blocks until the first
// completes, providing per-worker backpressure.
func NewShardedBatcher[T any](
	ctx context.Context,
	shards int,
	maxBatchSize int,
	flushInterval time.Duration,
	routeKey func(T) uint64,
	dedupKey func(T) any,
	process func(context.Context, []T) error,
	logger slog.Logger,
) *ShardedBatcher[T] {
	if shards < 1 {
		shards = 1
	}
	chans := make([]chan T, shards)
	for i := range chans {
		chans[i] = make(chan T, 4000)
		go runBatcher(ctx, chans[i], maxBatchSize, flushInterval, dedupKey, process, logger)
	}
	return &ShardedBatcher[T]{chans: chans, routeKey: routeKey}
}

func runBatcher[T any](
	ctx context.Context,
	ch <-chan T,
	maxBatchSize int,
	flushInterval time.Duration,
	dedupKey func(T) any,
	process func(context.Context, []T) error,
	logger slog.Logger,
) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	inFlight := make(chan struct{}, 1)
	batch := make(map[any]T)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		events := make([]T, 0, len(batch))
		for _, ev := range batch {
			events = append(events, ev)
		}
		batch = make(map[any]T)
		inFlight <- struct{}{}
		go func() {
			defer func() { <-inFlight }()
			if err := process(ctx, events); err != nil {
				logger.Error(ctx, "processing batch", slog.F("err", err))
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case ev := <-ch:
			batch[dedupKey(ev)] = ev
			for len(batch) < maxBatchSize {
				select {
				case ev := <-ch:
					batch[dedupKey(ev)] = ev
				default:
					goto drained
				}
			}
		drained:
			if len(batch) >= maxBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

type memberKey struct {
	UserID  int64
	GuildID int64
}
