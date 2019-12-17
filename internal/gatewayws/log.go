package gatewayws

import (
	"sync/atomic"
	"time"

	"cdr.dev/slog"
)

const LogInterval = 5 * time.Minute

func (s *Session) logTotalEvents() {
	var (
		t   = time.NewTicker(LogInterval)
		ctx = s.ctx
	)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}

		seq := atomic.LoadInt64(&s.seq)
		since := seq - s.last

		s.log.Info(
			s.ctx,
			"event report",
			slog.F("seq", seq),
			slog.F("events", since),
			slog.F("/sec", float64(since)/LogInterval.Seconds()),
		)

		s.last = seq
	}
}
