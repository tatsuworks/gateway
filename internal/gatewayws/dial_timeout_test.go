package gatewayws

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Both resume budgets are driven by noteDialFailure, which is only reached once
// a dial RETURNS. So a dial that never returns defeats the escape entirely: the
// read loop never finishes, Open never returns, the manager's reconnect loop
// stays parked inside it, and the shard is stuck exactly as in the incident.
//
// That is reachable. websocket.Dial with nil options uses http.DefaultClient,
// whose DefaultTransport bounds TCP connect (30s) and the TLS handshake (10s)
// but leaves ResponseHeaderTimeout unset — so a host that accepts the connection
// and then never sends response headers blocks forever. c.ctx carries no
// deadline of its own. Hence an explicit per-dial deadline.
func TestDialIsBoundedWhenTheHostAcceptsButNeverAnswers(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		// Accept the connection, then answer nothing at all.
		<-release
	}))
	// Cleanups run LIFO: unblock the handler before Close waits on it.
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	db := &shardInfoRecorder{}
	s, c := newResumableSession(t, db)
	s.resumeURL = srv.URL
	if !s.usingResumeURL() {
		t.Fatal("test setup must dial the resume URL")
	}

	done := make(chan struct{})
	var (
		err  error
		resp *http.Response
	)
	start := time.Now()
	go func() {
		defer close(done)
		_, resp, err = c.dialGatewayWithin(200 * time.Millisecond)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("dial did not return: an unbounded dial parks the reconnect loop forever")
	}

	if err == nil {
		t.Fatal("dial to a host that never answers returned no error")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("dial took %v, want it bounded by the supplied deadline", took)
	}

	// The deadline must present as a normal unanswered dial failure, not as our
	// own teardown: resp is nil (nothing replied, so it feeds the duration-based
	// no-response budget) and the CONNECTION context is still live, so the call
	// site's c.ctx.Err() == nil check still counts this against the resume host.
	if resp != nil {
		t.Fatalf("resp = %v, want nil so the failure counts as never-answered", resp)
	}
	if c.ctx.Err() != nil {
		t.Fatalf("connection context is %v: a dial deadline must not read as shutdown/Cancel, or the failure is never counted", c.ctx.Err())
	}
}
