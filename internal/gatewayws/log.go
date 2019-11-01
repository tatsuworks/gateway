package gatewayws

import (
	"sync/atomic"
	"time"

	"go.coder.com/slog"
)

func (s *Session) logTotalEvents() {
	var (
		t   = time.NewTicker(time.Minute)
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

		s.log.Info(
			s.ctx,
			"event report",
			slog.F("seq", seq),
			slog.F("/sec", (seq-s.last)/60),
		)

		s.last = seq
	}
}
