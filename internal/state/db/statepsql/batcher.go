package statepsql

import (
	"context"
	"time"

	"cdr.dev/slog"
)

// BatchEvent is implemented by event types to provide routing and dedup keys.
// RouteKey groups events onto the same worker (guildID); DedupKey collapses
// duplicate events within a batch (e.g. same member updated twice).
type BatchEvent interface {
	RouteKey() uint64
	DedupKey() uint64
}

type MemberEvent struct {
	GuildID int64
	UserID  int64
	Raw     []byte
	IsNew   bool
}

func (e MemberEvent) RouteKey() uint64 { return uint64(e.GuildID) }
func (e MemberEvent) DedupKey() uint64 { return uint64(e.UserID)<<32 | uint64(e.GuildID)&0xFFFFFFFF }

type PresenceEvent struct {
	GuildID int64
	UserID  int64
	Raw     []byte
}

func (e PresenceEvent) RouteKey() uint64 { return uint64(e.GuildID) }
func (e PresenceEvent) DedupKey() uint64 { return uint64(e.UserID)<<32 | uint64(e.GuildID)&0xFFFFFFFF }

type GuildEvent struct {
	GuildID int64
	Raw     []byte
}

func (e GuildEvent) RouteKey() uint64 { return uint64(e.GuildID) }
func (e GuildEvent) DedupKey() uint64 { return uint64(e.GuildID) }

// ShardedBatcher fans events across N independent batch workers, routed by
// RouteKey() % N. Events for the same guild cluster on one worker for better
// batch locality; dedup within a batch uses DedupKey().
type ShardedBatcher[T BatchEvent] struct {
	chans []chan T
}

func (s *ShardedBatcher[T]) Send(ctx context.Context, ev T) error {
	ch := s.chans[ev.RouteKey()%uint64(len(s.chans))]
	select {
	case ch <- ev:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func NewShardedBatcher[T BatchEvent](
	ctx context.Context,
	shards int,
	maxBatchSize int,
	flushInterval time.Duration,
	process func(context.Context, []T) error,
	logger slog.Logger,
) *ShardedBatcher[T] {
	if shards < 1 {
		shards = 1
	}
	chans := make([]chan T, shards)
	for i := range chans {
		chans[i] = make(chan T, 4000)
		go runBatcher(ctx, chans[i], maxBatchSize, flushInterval, process, logger)
	}
	return &ShardedBatcher[T]{chans: chans}
}

func runBatcher[T BatchEvent](
	ctx context.Context,
	ch <-chan T,
	maxBatchSize int,
	flushInterval time.Duration,
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
			batch[ev.DedupKey()] = ev
			for len(batch) < maxBatchSize {
				select {
				case ev := <-ch:
					batch[ev.DedupKey()] = ev
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
