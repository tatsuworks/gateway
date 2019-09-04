package gatewayws

import (
	"io"

	"golang.org/x/xerrors"
)

// readMessage populates buf on *Session with the next message.
func (s *Session) readMessage() error {
	s.buf.Reset()
	_, r, err := s.wsConn.Reader(s.ctx)
	if err != nil {
		return xerrors.Errorf("failed to get reader: %w", err)
	}

	_, err = io.Copy(s.buf, r)
	if err != nil {
		return xerrors.Errorf("failed to copy message: %w", err)
	}

	return nil
}
