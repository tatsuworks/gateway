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
// hash(keyOf(ev)) % N. Because the same key always lands on the same worker
// and each worker runs at most one flush at a time, two concurrent DB
// transactions can never contend on the same row — this avoids the
// transactionid waits we saw when the previous single-worker batcher allowed
// multiple overlapping upserts on the same (user_id, guild_id) key.
type ShardedBatcher[T any] struct {
	chans []chan T
	keyOf func(T) uint64
}

func (s *ShardedBatcher[T]) Send(ctx context.Context, ev T) error {
	ch := s.chans[s.keyOf(ev)%uint64(len(s.chans))]
	select {
	case ch <- ev:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NewShardedBatcher spawns `shards` batch workers, each with its own channel
// and in-flight flush slot. Events are deduplicated within a batch by
// keyOf(ev) — newer events for the same key overwrite older ones.
//
// Flushes happen when a worker's batch reaches maxBatchSize or every
// flushInterval. While a flush is in flight, the worker keeps draining its
// channel into the next batch; a second flush blocks until the first
// completes, providing per-shard backpressure.
func NewShardedBatcher[T any](
	ctx context.Context,
	shards int,
	maxBatchSize int,
	flushInterval time.Duration,
	keyOf func(T) uint64,
	process func(context.Context, []T) error,
	logger slog.Logger,
) *ShardedBatcher[T] {
	if shards < 1 {
		shards = 1
	}
	chans := make([]chan T, shards)
	for i := range chans {
		chans[i] = make(chan T, 4000)
		go runBatcher(ctx, chans[i], maxBatchSize, flushInterval, keyOf, process, logger)
	}
	return &ShardedBatcher[T]{chans: chans, keyOf: keyOf}
}

func runBatcher[T any](
	ctx context.Context,
	ch <-chan T,
	maxBatchSize int,
	flushInterval time.Duration,
	keyOf func(T) uint64,
	process func(context.Context, []T) error,
	logger slog.Logger,
) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	inFlight := make(chan struct{}, 1)
	batch := make(map[uint64]T)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		events := make([]T, 0, len(batch))
		for _, ev := range batch {
			events = append(events, ev)
		}
		batch = make(map[uint64]T)
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
			batch[keyOf(ev)] = ev
			for len(batch) < maxBatchSize {
				select {
				case ev := <-ch:
					batch[keyOf(ev)] = ev
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

// mixUserGuild hashes a (userID, guildID) pair for shard routing.
// The golden-ratio multiplier spreads Discord snowflake entropy across all
// bits so modulo-N sharding distributes evenly.
func mixUserGuild(userID, guildID int64) uint64 {
	return uint64(userID)*0x9E3779B97F4A7C15 ^ uint64(guildID)
}

func mixGuildID(guildID int64) uint64 {
	return uint64(guildID) * 0x9E3779B97F4A7C15
}
