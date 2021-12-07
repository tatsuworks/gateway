package gatewayws

import (
	"sync/atomic"
	"time"

	"cdr.dev/slog"
)

const LogInterval = 10 * time.Minute

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
			slog.F("write_queue", len(s.wch)),
			slog.F("waiting", s.state.WaitingQueries()),
			slog.F("state", s.curState),
		)
		s.persistStatus()
		s.last = seq
	}
}
