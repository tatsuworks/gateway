package gatewayws

import (
	"io"
	"time"

	"github.com/pkg/errors"
	"github.com/tatsuworks/gateway/discordetf"
)

func (s *Session) readHello() error {
	_, r, err := s.wsConn.Reader(s.ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get hello reader")
	}

	raw := s.bufs.Get()
	_, err = io.Copy(raw, r)
	if err != nil {
		s.bufs.Put(raw)
		return errors.Wrap(err, "failed to copy hello")
	}

	interval, trace, err := discordetf.DecodeHello(raw.B)
	if err != nil {
		return errors.Wrap(err, "failed to decode hello message")
	}

	s.bufs.Put(raw)
	s.interval = time.Duration(interval) * time.Millisecond
	s.trace = trace

	return nil
}
