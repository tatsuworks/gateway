package gatewayws

import (
	"context"
	"testing"
	"time"

	"cdr.dev/slog/sloggers/slogtest"
)

// parseEventReadTimeout maps the EVENT_READ_TIMEOUT env value (in seconds) to a
// per-read deadline for the main event loop. An unset or invalid value must
// preserve the historical 30s behavior; an explicit "0" disables the deadline
// so a healthy-but-idle connection (e.g. low-traffic staging) is not torn down
// on event silence, leaving liveness to the heartbeat-ACK watchdog.
func TestParseEventReadTimeout(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{"unset defaults to 30s", "", 30 * time.Second},
		{"explicit zero disables", "0", 0},
		{"positive value in seconds", "45", 45 * time.Second},
		{"non-numeric falls back to default", "garbage", 30 * time.Second},
		{"negative falls back to default", "-5", 30 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseEventReadTimeout(tc.raw); got != tc.want {
				t.Fatalf("parseEventReadTimeout(%q) = %v; want %v", tc.raw, got, tc.want)
			}
		})
	}
}

// readCtx with a disabled (<=0) timeout must hand back the connection context
// unchanged, with no deadline, so an idle read blocks until a real message or
// ctx cancellation rather than tripping a per-read deadline.
func TestReadCtxNoDeadlineWhenDisabled(t *testing.T) {
	s := &Session{log: slogtest.Make(t, nil)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := s.newConn(ctx, cancel)

	rctx, rcancel := c.readCtx(0)
	defer rcancel()
	if _, ok := rctx.Deadline(); ok {
		t.Fatal("readCtx(0) set a deadline; want none (rely on heartbeat watchdog)")
	}
}

// readCtx with a positive timeout must bound the read with a deadline roughly
// that far in the future.
func TestReadCtxHasDeadlineWhenSet(t *testing.T) {
	s := &Session{log: slogtest.Make(t, nil)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := s.newConn(ctx, cancel)

	rctx, rcancel := c.readCtx(45 * time.Second)
	defer rcancel()
	dl, ok := rctx.Deadline()
	if !ok {
		t.Fatal("readCtx(45s) set no deadline; want one")
	}
	if remaining := time.Until(dl); remaining < 40*time.Second || remaining > 45*time.Second {
		t.Fatalf("readCtx(45s) deadline %v out from now; want ~45s", remaining)
	}
}
