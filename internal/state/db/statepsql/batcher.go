package statepsql

import (
	"context"
	"time"

	"cdr.dev/slog"
)

// BatchEvent is implemented by event types to provide routing and dedup keys.
// RouteKey groups events onto the same worker (guildID); DedupKey collapses
// duplicate events within a batch (e.g. same member updated twice).
//
// DedupKey returns any so composite keys can be a comparable struct/array
// without lossy bit-packing of two 64-bit Discord snowflakes.
type BatchEvent interface {
	RouteKey() uint64
	DedupKey() any
}

type MemberEvent struct {
	GuildID int64
	UserID  int64
	Raw     []byte
	IsNew   bool
}

func (e MemberEvent) RouteKey() uint64 { return uint64(e.GuildID) }
func (e MemberEvent) DedupKey() any    { return [2]int64{e.UserID, e.GuildID} }

type PresenceEvent struct {
	GuildID int64
	UserID  int64
	Raw     []byte
}

func (e PresenceEvent) RouteKey() uint64 { return uint64(e.GuildID) }
func (e PresenceEvent) DedupKey() any    { return [2]int64{e.UserID, e.GuildID} }

type GuildEvent struct {
	GuildID int64
	Raw     []byte
}

func (e GuildEvent) RouteKey() uint64 { return uint64(e.GuildID) }
func (e GuildEvent) DedupKey() any    { return e.GuildID }

// flushRequest asks a worker to synchronously flush the events for a single
// route key and report that flush's error. The route key is carried so the
// worker flushes only that key's events, not the whole (multi-guild) batch.
type flushRequest struct {
	routeKey uint64
	reply    chan error
}

// ShardedBatcher fans events across N independent batch workers, routed by
// RouteKey() % N. Events for the same guild cluster on one worker for better
// batch locality; dedup within a batch uses DedupKey().
type ShardedBatcher[T BatchEvent] struct {
	chans      []chan T
	flushChans []chan flushRequest
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

// FlushForShard synchronously flushes the events currently queued for routeKey,
// blocking until those rows are persisted, and returns that flush's error.
// Callers use it as a durability barrier — e.g. CompleteGuildBackfill flushes
// before stamping backfilled_at, so a just-queued member upsert is durable (and
// a failed flush aborts the stamp) rather than the marker being written over
// un-persisted rows. It flushes only routeKey's events (other guilds sharing the
// worker keep their queued rows), so one guild's flush failure never drops
// another's. It flushes only events whose Send has already returned, so the
// caller must Send before calling this.
func (s *ShardedBatcher[T]) FlushForShard(ctx context.Context, routeKey uint64) error {
	req := flushRequest{routeKey: routeKey, reply: make(chan error, 1)}
	fc := s.flushChans[routeKey%uint64(len(s.flushChans))]
	select {
	case fc <- req:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-req.reply:
		return err
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
	flushChans := make([]chan flushRequest, shards)
	for i := range chans {
		chans[i] = make(chan T, 4000)
		flushChans[i] = make(chan flushRequest)
		go runBatcher(ctx, chans[i], flushChans[i], maxBatchSize, flushInterval, process, logger)
	}
	return &ShardedBatcher[T]{chans: chans, flushChans: flushChans}
}

func runBatcher[T BatchEvent](
	ctx context.Context,
	ch <-chan T,
	flushReq <-chan flushRequest,
	maxBatchSize int,
	flushInterval time.Duration,
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
			// Wait for the in-flight write (kicked off by flush above, or a
			// prior tick) to complete before exiting so the final batch isn't
			// dropped on shutdown.
			inFlight <- struct{}{}
			return
		case ev := <-ch:
			batch[ev.DedupKey()] = ev
			// Bound the drain by iteration count, not just unique keys.
			// Under sustained hot-key traffic that dedupes to fewer than
			// maxBatchSize keys, an unbounded inner loop would never fall
			// back to the outer select, starving the ticker and ctx.Done()
			// cases.
			for i := 1; i < maxBatchSize && len(batch) < maxBatchSize; i++ {
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
		case req := <-flushReq:
			// Pull everything currently buffered for this worker into the batch
			// so the flush sees every event queued before the request arrived
			// (their Send has already returned).
		drainBuffered:
			for {
				select {
				case ev := <-ch:
					batch[ev.DedupKey()] = ev
				default:
					break drainBuffered
				}
			}
			// Wait for any in-flight async write to finish, then synchronously
			// write only THIS route key's events, leaving other guilds' rows
			// queued. Flushing the whole worker here would let a failure while
			// completing one guild drop another guild's co-queued members, which
			// its later completion would then stamp over.
			inFlight <- struct{}{}
			var events []T
			for k, ev := range batch {
				if ev.RouteKey() == req.routeKey {
					events = append(events, ev)
					delete(batch, k)
				}
			}
			var ferr error
			if len(events) > 0 {
				ferr = process(ctx, events)
			}
			<-inFlight
			req.reply <- ferr
		}
	}
}
