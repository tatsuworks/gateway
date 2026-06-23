package gatewayws

import (
	"context"
	"time"

	"cdr.dev/slog"
	"golang.org/x/xerrors"

	"github.com/tatsuworks/czlib"
)

const connectionTimeout = 30

// readMessage populates buf on *conn with the next message.
func (c *conn) readMessage() error {
	start := time.Now()
	defer func() {
		took := time.Since(start)
		if took > connectionTimeout*time.Second {
			c.s.log.Error(c.ctx, "took too long to get reader", slog.F("took", time.Since(start).String()))
		}
	}()

	ctx, cancel := context.WithTimeout(c.ctx, connectionTimeout*time.Second)
	defer cancel()

	_, r, err := c.wsConn.Reader(ctx)
	if err != nil {
		return xerrors.Errorf("get ws reader: %w", err)
	}

	c.zr.(czlib.Resetter).Reset(r)
	defer c.zr.Close()

	_, err = c.buf.ReadFrom(c.zr)
	if err != nil {
		return xerrors.Errorf("copy message: %w", err)
	}

	return nil
}
