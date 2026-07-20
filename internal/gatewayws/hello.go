package gatewayws

import (
	"bytes"
	"time"

	"golang.org/x/xerrors"
)

func (c *conn) readHello() error {
	c.buf = c.s.bufferPool.Get().(*bytes.Buffer)
	defer c.cleanupBuffer()

	// The handshake runs before the heartbeat watchdog starts, so it always uses
	// the fixed connectionTimeout to fail fast on a broken connect.
	err := c.readMessage(connectionTimeout * time.Second)
	if err != nil {
		return xerrors.Errorf("read message: %w", err)
	}

	interval, trace, err := c.s.enc.DecodeHello(c.buf.Bytes())
	if err != nil {
		return xerrors.Errorf("decode hello message: %w", err)
	}
	if interval <= 0 {
		return xerrors.Errorf("invalid interval received: %d", interval)
	}
	c.interval = time.Duration(interval) * time.Millisecond
	c.trace = trace

	return nil
}
