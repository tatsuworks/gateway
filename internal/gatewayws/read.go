package gatewayws

import (
	"bytes"
	"context"
	"time"

	"cdr.dev/slog"
	"golang.org/x/xerrors"

	"github.com/tatsuworks/czlib"
)

// readMessage populates buf on *Session with the next message.
func (s *Session) readMessage() error {
	s.buf = s.bufferPool.Get().(*bytes.Buffer)
	start := time.Now()
	defer func() {
		took := time.Since(start)
		if took > 20*time.Second {
			s.log.Error(s.ctx, "took more than 20s to get reader", slog.F("took", time.Since(start).String()))
		}
	}()

	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
	defer cancel()

	_, r, err := s.wsConn.Reader(ctx)
	if err != nil {
		return xerrors.Errorf("get ws reader: %w", err)
	}

	s.zr.(czlib.Resetter).Reset(r)

	_, err = s.buf.ReadFrom(s.zr)
	if err != nil {
		return xerrors.Errorf("copy message: %w", err)
	}

	return nil
}
