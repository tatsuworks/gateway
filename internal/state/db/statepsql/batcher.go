package statepsql

import (
	"context"
	"sync"
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

// flushRequest asks a worker to synchronously flush and report the result for a
// specific route key. The route key is carried (not just the worker index) so
// the worker can surface a dropped-flush error attributed to that exact key.
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

// FlushForShard synchronously flushes every event currently queued for the
// worker that owns routeKey, blocking until those rows are persisted. Callers
// use it as a durability barrier — e.g. before stamping a backfill complete —
// so a just-queued upsert is guaranteed written before a dependent action on
// the same key. It also surfaces (and clears) any error dropped by an earlier
// async (ticker- or size-triggered) flush that included routeKey, so a
// transient write failure cannot be silently stamped over. The error is
// attributed to routeKey specifically, so a flush for one key never consumes
// another key's failure even when they share a worker. It flushes only events
// whose Send has already returned, so the caller must Send before calling this.
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

// BatcherOption configures optional ShardedBatcher behavior.
type BatcherOption func(*batcherConfig)

type batcherConfig struct {
	trackFlushErrors bool
}

// WithFlushErrorTracking makes workers retain per-route-key errors from dropped
// (async, or synchronous flushReq) flushes so a later FlushForShard for that key
// surfaces them. Enable it only for batchers whose FlushForShard results are
// consumed — i.e. the member batcher backing CompleteGuildBackfill. Without a
// consumer the retained entries are never read and would accumulate for the life
// of the process during a write outage, so it stays off by default.
func WithFlushErrorTracking() BatcherOption {
	return func(c *batcherConfig) { c.trackFlushErrors = true }
}

func NewShardedBatcher[T BatchEvent](
	ctx context.Context,
	shards int,
	maxBatchSize int,
	flushInterval time.Duration,
	process func(context.Context, []T) error,
	logger slog.Logger,
	opts ...BatcherOption,
) *ShardedBatcher[T] {
	if shards < 1 {
		shards = 1
	}
	var cfg batcherConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	chans := make([]chan T, shards)
	flushChans := make([]chan flushRequest, shards)
	for i := range chans {
		chans[i] = make(chan T, 4000)
		flushChans[i] = make(chan flushRequest)
		go runBatcher(ctx, chans[i], flushChans[i], maxBatchSize, flushInterval, process, logger, cfg)
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
	cfg batcherConfig,
) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	inFlight := make(chan struct{}, 1)
	batch := make(map[any]T)

	// failed records the last dropped-flush error per route key. A batch spans
	// many route keys (this worker owns every key with key%shards == this
	// shard), and a failed flush drops all of them without retry, so the error
	// must be attributed to each affected key rather than latched worker-wide.
	// Otherwise the first FlushForShard for any key on this worker would consume
	// and clear the failure, and a different key whose members were in the same
	// dropped batch would then flush clean and stamp its backfill complete over
	// the missing rows. Guarded by failedMu because the async flush goroutine
	// writes it while the batcher loop reads it. FlushForShard clears a key on
	// read; the entry is re-created if a later flush for that key fails again.
	// Only populated when trackFlushErrors is set (batchers with a FlushForShard
	// consumer); otherwise entries would never be read and would accumulate.
	var (
		failedMu sync.Mutex
		failed   = make(map[uint64]error)
	)
	markFailed := func(events []T, err error) {
		if !cfg.trackFlushErrors {
			return
		}
		failedMu.Lock()
		for _, ev := range events {
			failed[ev.RouteKey()] = err
		}
		failedMu.Unlock()
	}

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
				markFailed(events, err)
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
			// Pull everything currently buffered for this worker into the
			// batch so the synchronous flush covers all events queued before
			// the request arrived (their Send has already returned).
		drainBuffered:
			for {
				select {
				case ev := <-ch:
					batch[ev.DedupKey()] = ev
				default:
					break drainBuffered
				}
			}
			// Wait for any in-flight async write to finish, then write the
			// current batch synchronously so the rows are durable before
			// FlushForShard returns to the caller.
			inFlight <- struct{}{}
			var ferr error
			if len(batch) > 0 {
				events := make([]T, 0, len(batch))
				for _, ev := range batch {
					events = append(events, ev)
				}
				batch = make(map[any]T)
				if ferr = process(ctx, events); ferr != nil {
					// This synchronous batch also spans multiple route keys;
					// on failure record every one so a key other than the
					// caller's does not later flush clean over dropped rows.
					markFailed(events, ferr)
				}
			}
			<-inFlight
			// Surface (and clear) a dropped-flush error for the caller's route
			// key only — from this synchronous flush or an earlier async one for
			// the same key. The inFlight barrier above guarantees any async
			// goroutine has finished, so failed reflects its final result.
			failedMu.Lock()
			if e, bad := failed[req.routeKey]; bad {
				delete(failed, req.routeKey)
				if ferr == nil {
					ferr = e
				}
			}
			failedMu.Unlock()
			req.reply <- ferr
		}
	}
}
