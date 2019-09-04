package gatewayws

import (
	"time"

	"github.com/tatsuworks/gateway/discordetf"
	"golang.org/x/xerrors"
)

func (s *Session) readHello() error {
	err := s.readMessage()
	if err != nil {
		return xerrors.Errorf("failed to read message: %w", err)
	}

	interval, trace, err := discordetf.DecodeHello(s.buf.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to decode hello message: %w", err)
	}

	s.interval = time.Duration(interval) * time.Millisecond
	s.trace = trace

	return nil
}
