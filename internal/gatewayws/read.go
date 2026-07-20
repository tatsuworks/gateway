package gatewayws

import (
	"context"
	"strconv"
	"time"

	"cdr.dev/slog"
	"golang.org/x/xerrors"

	"github.com/tatsuworks/czlib"
)

// connectionTimeout bounds the HELLO handshake read (in seconds). The handshake
// runs before the heartbeat watchdog starts, so it always needs a deadline to
// fail fast on a broken connect, regardless of the tunable event-read timeout.
const connectionTimeout = 30

// defaultEventReadTimeout is the main event loop's per-read deadline when
// EVENT_READ_TIMEOUT is unset or invalid — matching the historical behavior.
const defaultEventReadTimeout = connectionTimeout * time.Second

// parseEventReadTimeout maps the EVENT_READ_TIMEOUT env value (in seconds) to a
// per-read deadline for the main event loop. Unset or invalid preserves the
// historical default; an explicit "0" disables the deadline so a healthy-but-
// idle connection (low-traffic environments) is not torn down on event silence,
// leaving liveness to the heartbeat-ACK watchdog (see sendHeartbeats).
func parseEventReadTimeout(raw string) time.Duration {
	if raw == "" {
		return defaultEventReadTimeout
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return defaultEventReadTimeout
	}
	return time.Duration(n) * time.Second
}

// readCtx returns the context for a single read: a deadline-bounded child of
// c.ctx when timeout > 0, or c.ctx unchanged (no deadline) when timeout <= 0.
func (c *conn) readCtx(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return c.ctx, func() {}
	}
	return context.WithTimeout(c.ctx, timeout)
}

// readMessage populates buf on *conn with the next message. timeout bounds the
// wait for a reader; timeout <= 0 means no per-read deadline.
func (c *conn) readMessage(timeout time.Duration) error {
	start := time.Now()
	defer func() {
		took := time.Since(start)
		if timeout > 0 && took > timeout {
			c.s.log.Error(c.ctx, "took too long to get reader", slog.F("took", took.String()))
		}
	}()

	ctx, cancel := c.readCtx(timeout)
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
