package gatewayws

import (
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/tatsuworks/gateway/etf"
)

// This is a magic number I stole from discordgo
const FailedHeartbeatAcks = 5

type heartbeatOp struct {
	Op   int   `json:"op"`
	Data int64 `json:"d"`
}

func (s *Session) heartbeat() error {
	var (
		buf = s.bufs.Get()
		c   = new(etf.Context)
	)

	defer s.bufs.Put(buf)
	err := c.Write(buf, heartbeatOp{
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

	_, err = w.Write(buf.B)
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
			if s.lastAck.Sub(s.lastHB) > s.interval*5 {
				s.log.Warn("no response to heartbeats, closing")
				cancel()
				return
			}
		}

		err := s.heartbeat()
		if err != nil {
			s.log.Error("failed to send heartbeat", zap.Error(err))
			cancel()
			return
		}

		s.log.Info("sent heartbeat")
		s.lastHB = time.Now()
	}
}
