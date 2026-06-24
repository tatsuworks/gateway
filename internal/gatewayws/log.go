package gatewayws

import (
	"sync/atomic"
	"time"

	"cdr.dev/slog"
)

const (
	LogInterval    = 10 * time.Minute
	StatusInterval = 1 * time.Minute
)

func (c *conn) logTotalEvents() {
	var (
		logT    = time.NewTicker(LogInterval)
		statusT = time.NewTicker(StatusInterval)
		ctx     = c.ctx
	)
	defer logT.Stop()
	defer statusT.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-logT.C:
			seq := atomic.LoadInt64(&c.s.seq)
			since := seq - atomic.LoadInt64(&c.s.last)
			curState, _, _, _ := c.snapshot()

			c.s.log.Info(
				c.ctx,
				"event report",
				slog.F("seq", seq),
				slog.F("events", since),
				slog.F("/sec", float64(since)/LogInterval.Seconds()),
				slog.F("write_queue", len(c.wch)),
				slog.F("waiting", c.s.state.WaitingQueries()),
				slog.F("state", curState),
			)
			c.s.persistStatus(curState)
			atomic.StoreInt64(&c.s.last, seq)
		case <-statusT.C:
			curState, _, _, _ := c.snapshot()
			c.s.persistStatus(curState)
		}
	}
}
