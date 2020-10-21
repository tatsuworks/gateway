package gatewayws

import (
	"context"
	"time"

	"cdr.dev/slog"
	"golang.org/x/xerrors"

	"github.com/tatsuworks/czlib"
)

const connectionTimeout = 10

// readMessage populates buf on *Session with the next message.
func (s *Session) readMessage() error {
	start := time.Now()
	defer func() {
		took := time.Since(start)
		if took > connectionTimeout*time.Second {
			s.log.Error(s.ctx, "took too long to get reader", slog.F("took", time.Since(start).String()))
		}
	}()

	ctx, cancel := context.WithTimeout(s.ctx, connectionTimeout*time.Second)
	defer cancel()

	_, r, err := s.wsConn.Reader(ctx)
	if err != nil {
		return xerrors.Errorf("get ws reader: %w", err)
	}

	s.zr.(czlib.Resetter).Reset(r)
	defer s.zr.Close()

	_, err = s.buf.ReadFrom(s.zr)
	if err != nil {
		return xerrors.Errorf("copy message: %w", err)
	}

	return nil
}
