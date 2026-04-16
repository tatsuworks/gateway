package statepsql

import (
	"context"
	"time"

	"cdr.dev/slog"
)

type memberKey struct {
	UserID  int64
	GuildID int64
}
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

// BatchWorker starts a goroutine that deduplicates and batches events before
// writing them to the database. Events are keyed by (UserID, GuildID) for
// members/presences or by GuildID for guilds — newer events for the same key
// overwrite older ones, reducing redundant writes.
//
// Flushes happen when the batch reaches maxBatchSize or every flushInterval,
// whichever comes first. Flushes run in a separate goroutine so the batcher
// keeps draining the channel while the DB write is in flight, allowing
// multiple batches to be written concurrently across the connection pool.
func BatchWorker[T any](
	ctx context.Context,
	maxBatchSize int,
	flushInterval time.Duration,
	process func(context.Context, []T) error,
	logger slog.Logger,
) chan<- T {
	ch := make(chan T, 4000)

	go func() {
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()

		batch := make(map[any]T)

		// flush snapshots the current batch and processes it in a background
		// goroutine so the main loop can continue draining without waiting
		// for the DB write to complete.
		flush := func() {
			if len(batch) == 0 {
				return
			}
			events := make([]T, 0, len(batch))
			for _, ev := range batch {
				events = append(events, ev)
			}
			batch = make(map[any]T)
			go func() {
				if err := process(ctx, events); err != nil {
					logger.Error(ctx, "processing batch", slog.F("err", err))
				}
			}()
		}

		// addToBatch deduplicates by composite key — only the latest event
		// for a given key is kept in the batch.
		addToBatch := func(ev T) {
			var key any
			switch v := any(ev).(type) {
			case MemberEvent:
				key = memberKey{v.UserID, v.GuildID}
			case PresenceEvent:
				key = memberKey{v.UserID, v.GuildID}
			case GuildEvent:
				key = v.GuildID
			default:
				key = v
			}
			batch[key] = ev
		}

		for {
			select {
			case <-ctx.Done():
				flush()
				return
			case ev := <-ch:
				addToBatch(ev)
				// Non-blocking drain: pull all buffered events from the
				// channel before deciding to flush. This catches up quickly
				// after a flush blocks, and maximises deduplication.
				for len(batch) < maxBatchSize {
					select {
					case ev := <-ch:
						addToBatch(ev)
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
	}()

	return ch
}
