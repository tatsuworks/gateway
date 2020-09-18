package gatewayws

import (
	"bytes"
	"time"

	"golang.org/x/xerrors"
)

func (s *Session) readHello() error {
	s.buf = s.bufferPool.Get().(*bytes.Buffer)
	defer s.cleanupBuffer()

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
