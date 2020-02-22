package gatewayws

import (
	"golang.org/x/xerrors"

	"github.com/tatsuworks/czlib"
)

// readMessage populates buf on *Session with the next message.
func (s *Session) readMessage() error {
	s.buf.Reset()
	_, r, err := s.wsConn.Reader(s.ctx)
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
