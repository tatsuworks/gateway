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

// BatchWorker starts a goroutine that batches events and processes them.
func BatchWorker[T any](
	ctx context.Context,
	maxBatchSize int,
	flushInterval time.Duration,
	process func(context.Context, []T) error,
	logger slog.Logger, // Use your logger interface/type
) chan<- T {
	ch := make(chan T, 4000)

	go func() {
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()

		batch := make(map[any]T)

		flush := func() {
			if len(batch) == 0 {
				return
			}
			events := make([]T, 0, len(batch))
			for _, ev := range batch {
				events = append(events, ev)
			}
			if err := process(ctx, events); err != nil {
				logger.Error(ctx, "processing batch", slog.F("err", err))
			}
			batch = make(map[any]T)
		}

		for {
			select {
			case <-ctx.Done():
				flush()
				return
			case ev := <-ch:
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
