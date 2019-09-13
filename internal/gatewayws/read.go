package gatewayws

import (
	"io"

	"golang.org/x/xerrors"

	"github.com/tatsuworks/czlib"
)

// readMessage populates buf on *Session with the next message.
func (s *Session) readMessage() error {
	s.buf.Reset()
	_, wr, err := s.wsConn.Reader(s.ctx)
	if err != nil {
		return xerrors.Errorf("failed to get ws reader: %w", err)
	}

	sr, err := czlib.NewStreamReader(wr, s.strm)
	if err != nil {
		return xerrors.Errorf("failed to get zlib reader: %w", err)
	}

	_, err = io.Copy(s.buf, sr)
	if err != nil {
		return xerrors.Errorf("failed to copy message: %w", err)
	}

	return nil
}
