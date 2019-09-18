package gatewayws

import (
	"sync/atomic"
	"time"

	"go.uber.org/zap"
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
			"event report",
			zap.Int64("seq", seq),
			zap.Int64("/sec", (seq-s.last)/60),
		)

		s.last = seq
	}
}
