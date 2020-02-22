package gatewayws

import (
	"time"

	"golang.org/x/xerrors"
)

func (s *Session) readHello() error {
	err := s.readMessage()
	if err != nil {
		return xerrors.Errorf("read message: %w", err)
	}

	interval, trace, err := s.enc.DecodeHello(s.buf.Bytes())
	if err != nil {
		return xerrors.Errorf("decode hello message: %w", err)
	}

	s.interval = time.Duration(interval) * time.Millisecond
	s.trace = trace

	return nil
}
