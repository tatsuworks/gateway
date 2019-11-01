package gatewayws

import (
	"sync/atomic"
	"time"

	"go.coder.com/slog"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/tatsuworks/gateway/etf"
)

type heartbeatOp struct {
	Op   int   `json:"op"`
	Data int64 `json:"d"`
}

func (s *Session) heartbeat() error {
	var c = new(etf.Context)

	s.hbuf.Reset()
	err := c.Write(s.hbuf, heartbeatOp{
		Op:   1,
		Data: atomic.LoadInt64(&s.seq),
	})
	if err != nil {
		return xerrors.Errorf("failed to write heartbeat: %w", err)
	}

	w, err := s.wsConn.Writer(s.ctx, websocket.MessageBinary)
	if err != nil {
		return xerrors.Errorf("failed to get heartbeat writer: %w", err)
	}
	defer w.Close()

	_, err = w.Write(s.hbuf.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to copy heartbeat: %w", err)
	}

	return nil
}

func (s *Session) sendHeartbeats() {
	var (
		t      = time.NewTicker(s.interval)
		ctx    = s.ctx
		cancel = s.cancel
	)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}

		if !s.lastHB.IsZero() {
			if s.lastAck.Sub(s.lastHB) > s.interval {
				s.log.Warn(s.ctx, "no response to heartbeat, closing")
				cancel()
				return
			}
		}

		err := s.heartbeat()
		if err != nil {
			s.log.Error(s.ctx, "failed to send heartbeat", slog.Error(err))
			cancel()
			return
		}

		s.lastHB = time.Now()
	}
}
