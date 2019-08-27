package gatewayws

import (
	"io"

	"golang.org/x/xerrors"
)

func (s *Session) readMessage() ([]byte, error) {
	_, r, err := s.wsConn.Reader(s.ctx)
	if err != nil {
		return nil, xerrors.Errorf("failed to get reader: %w", err)
	}

	raw := s.bufs.Get()
	_, err = io.Copy(raw, r)
	if err != nil {
		s.bufs.Put(raw)
		return nil, xerrors.Errorf("failed to copy message: %w", err)
	}

	return raw.B, nil
}
