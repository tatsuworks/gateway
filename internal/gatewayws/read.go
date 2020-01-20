package gatewayws

import (
	"io"

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

	resetter, ok := s.zr.(czlib.Resetter)
	if !ok {
		return xerrors.Errorf("reset zlib reader")
	}
	resetter.Reset(r)

	_, err = io.Copy(s.buf, s.zr)
	if err != nil {
		return xerrors.Errorf("copy message: %w", err)
	}

	return nil
}
