package gatewayws

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The whole answered budget rests on one line at the dial site: resp != nil
// means "the host answered and refused the upgrade". Every policy test drives
// noteDialFailure with a hand-chosen kind, so they pin the policy and say
// nothing about the classification. That leaves the incident's own signature —
// a 503 at the WebSocket handshake — untested end to end: a websocket-library
// upgrade that changed the return contract, or a refactor of the call site,
// would route every 503 onto the hour-long silence budget and strand the shard
// again with the suite still green.
func TestAnsweredRejectionIsClassifiedAsRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// What a retired Discord edge does: answer the upgrade, refuse it.
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	db := &shardInfoRecorder{}
	s, c := newResumableSession(t, db)
	s.resumeURL = srv.URL
	if !s.usingResumeURL() {
		t.Fatal("test setup must dial the resume URL")
	}

	_, resp, err := c.dialGatewayWithin(5 * time.Second)
	if err == nil {
		t.Fatal("dial against a 503 succeeded")
	}
	if resp == nil {
		t.Fatal("resp = nil for an answered rejection: every 503 would land on the silence budget and strand the shard")
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("resp.StatusCode = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	if got := classifyDialFailure(resp); got != dialRefused {
		t.Fatalf("classifyDialFailure(503) = %v, want dialRefused", got)
	}

	// And the classification actually drives the escape: the same dial, repeated
	// past both thresholds, clears the tuple.
	start := time.Now()
	for i := 0; i < MaxResumeDialFailures; i++ {
		_, resp, err := c.dialGatewayWithin(5 * time.Second)
		if err == nil {
			t.Fatal("dial against a 503 succeeded")
		}
		s.noteDialFailureAt(start.Add(time.Duration(i)*MinResumeDialFailureSpan), true, classifyDialFailure(resp))
	}
	if s.shouldResume() {
		t.Fatal("a host answering 503 to every dial did not discard the resume tuple")
	}
}

// 429 is an answer too, but it is the one answer that is never host-specific.
// Pin that it is classified apart from a refusal at the dial level, not just in
// the policy tests.
func TestThrottledRejectionIsClassifiedApartFromRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	db := &shardInfoRecorder{}
	s, c := newResumableSession(t, db)
	s.resumeURL = srv.URL

	_, resp, err := c.dialGatewayWithin(5 * time.Second)
	if err == nil {
		t.Fatal("dial against a 429 succeeded")
	}
	if resp == nil {
		t.Fatal("resp = nil for a 429; the host answered")
	}
	if got := classifyDialFailure(resp); got != dialThrottled {
		t.Fatalf("classifyDialFailure(429) = %v, want dialThrottled", got)
	}
}

// The nil direction, asserted against the classifier rather than only against
// the dial (TestDialIsBoundedWhenTheHostAcceptsButNeverAnswers covers the dial).
func TestNoResponseIsClassifiedAsUnanswered(t *testing.T) {
	if got := classifyDialFailure(nil); got != dialUnanswered {
		t.Fatalf("classifyDialFailure(nil) = %v, want dialUnanswered", got)
	}
}
