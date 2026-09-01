package gatewayws

import (
	"bytes"
	"os"
	"sync/atomic"
	"testing"

	"github.com/tatsuworks/gateway/discord"
)

// stubEncoding supplies just the piece of discord.Encoding that GatewayURL
// needs (the query-string encoding name); the decode methods are never reached
// in these tests, so the interface is embedded rather than implemented.
type stubEncoding struct {
	discord.Encoding
}

func (stubEncoding) Name() string { return "etf" }

const mainGatewayURL = "wss://gateway.discord.gg/"

// A shard that persists a resume_gateway_url pointing at a Discord edge which
// no longer serves it fails at the WebSocket *handshake* (503), before
// IDENTIFY/RESUME is ever sent — so Discord never gets to tell us the session
// is bad via INVALID_SESSION. Without a dial-stage failure counter the shard
// redials the same dead host forever. These tests pin the escape hatch:
// consecutive dial failures against a resume URL discard the resume tuple, so
// the next connect falls back to the main gateway URL.

// Below the threshold the resume tuple is still the best guess (a single dial
// failure is far more likely to be a transient blip than a retired edge), so it
// must survive untouched and unpersisted.
func TestResumeDialFailuresBelowThresholdKeepResumeState(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	for i := 0; i < MaxResumeDialFailures-1; i++ {
		s.noteDialFailure(true)
	}

	if !s.shouldResume() {
		t.Fatalf("resume state discarded after %d failures; threshold is %d",
			MaxResumeDialFailures-1, MaxResumeDialFailures)
	}
	if s.resumeURL == "" {
		t.Fatal("resumeURL cleared below the failure threshold")
	}
	if db.calls != 0 {
		t.Fatalf("persisted shard info below threshold (calls=%d), want 0", db.calls)
	}
}

// At the threshold the tuple is discarded in memory *and* persisted cleared, so
// a pod restart cannot reload the dead host out of the shards table and walk
// straight back into the same loop.
func TestResumeDialFailuresAtThresholdDiscardResumeState(t *testing.T) {
	db := &shardInfoRecorder{}
	s, c := newResumableSession(t, db)

	for i := 0; i < MaxResumeDialFailures; i++ {
		s.noteDialFailure(true)
	}

	if s.shouldResume() {
		t.Fatal("session still resumable after the dial-failure threshold")
	}
	if got := atomic.LoadInt64(&s.seq); got != 0 {
		t.Fatalf("seq = %d, want 0", got)
	}
	if s.sessID != "" || s.resumeURL != "" {
		t.Fatalf("sessID=%q resumeURL=%q, want both empty", s.sessID, s.resumeURL)
	}
	if db.calls != 1 {
		t.Fatalf("SetShardInfo calls = %d, want exactly 1 (persist the cleared tuple once)", db.calls)
	}
	if db.seq != 0 || db.sess != "" || db.resumeURL != "" {
		t.Fatalf("persisted tuple = seq:%d sess:%q resumeURL:%q, want cleared",
			db.seq, db.sess, db.resumeURL)
	}
	if got := c.GatewayURL(); got[:len(mainGatewayURL)] != mainGatewayURL {
		t.Fatalf("GatewayURL() = %q, want the main gateway URL after invalidation", got)
	}
}

// The counter must be consecutive: a dial that connects clears it, so unrelated
// transient failures spread over hours never add up to a spurious invalidation.
func TestDialSuccessResetsResumeDialFailures(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	for i := 0; i < MaxResumeDialFailures-1; i++ {
		s.noteDialFailure(true)
	}
	s.noteDialSuccess()
	for i := 0; i < MaxResumeDialFailures-1; i++ {
		s.noteDialFailure(true)
	}

	if !s.shouldResume() {
		t.Fatal("resume state discarded; a successful dial must reset the failure counter")
	}
	if db.calls != 0 {
		t.Fatalf("persisted shard info (calls=%d), want 0", db.calls)
	}
}

// Only dial failures against a resume URL indicate a bad resume host. A shard
// already dialing the main gateway URL has nothing to invalidate, so those
// failures must not be counted (and must not clear an unrelated tuple).
func TestDialFailuresAgainstMainURLDoNotInvalidate(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	for i := 0; i < MaxResumeDialFailures*3; i++ {
		s.noteDialFailure(false)
	}

	if !s.shouldResume() {
		t.Fatal("resume state discarded by failures that did not use the resume URL")
	}
	if db.calls != 0 {
		t.Fatalf("persisted shard info (calls=%d), want 0", db.calls)
	}
}

// Once invalidated the counter starts over, so the shard does not immediately
// re-clear (and re-persist) an already-empty tuple on every subsequent failure.
func TestResumeDialFailuresResetAfterInvalidation(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	for i := 0; i < MaxResumeDialFailures; i++ {
		s.noteDialFailure(true)
	}
	if db.calls != 1 {
		t.Fatalf("SetShardInfo calls = %d after invalidation, want 1", db.calls)
	}

	// The tuple is empty now, so subsequent dials use the main URL and report
	// usedResumeURL=false; nothing further should be persisted.
	for i := 0; i < MaxResumeDialFailures*2; i++ {
		s.noteDialFailure(false)
	}
	if db.calls != 1 {
		t.Fatalf("SetShardInfo calls = %d, want 1 (no repeat persists of an empty tuple)", db.calls)
	}
}

// usingResumeURL is what the dial site uses to classify a failure; it must
// agree with GatewayURL's own resume-vs-main decision.
func TestUsingResumeURLMatchesGatewayURL(t *testing.T) {
	db := &shardInfoRecorder{}
	s, c := newResumableSession(t, db)

	if !s.usingResumeURL() {
		t.Fatal("usingResumeURL() = false for a fully populated resume tuple")
	}
	if got := c.GatewayURL(); got[:len(s.resumeURL)] != s.resumeURL {
		t.Fatalf("GatewayURL() = %q, want the resume URL prefix %q", got, s.resumeURL)
	}

	// A resume URL with no session ID is not resumable and must not be dialed.
	s.sessID = ""
	if s.usingResumeURL() {
		t.Fatal("usingResumeURL() = true with an empty session ID")
	}
	if got := c.GatewayURL(); got[:len(mainGatewayURL)] != mainGatewayURL {
		t.Fatalf("GatewayURL() = %q, want the main gateway URL", got)
	}
}

// Discarding the tuple is a signal the manager needs: the next dial targets the
// main gateway URL, a different host from the one the failures came from, so
// the reconnect backoff earned against the dead edge must not be applied to it.
// Measured on staging before this: 9.0s of a 15.8s escape was that dead wait.
func TestInvalidationFlagsResumeDiscarded(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	for i := 0; i < MaxResumeDialFailures-1; i++ {
		s.noteDialFailure(true)
	}
	if s.TakeResumeDiscarded() {
		t.Fatal("reported a discard below the failure threshold")
	}

	s.noteDialFailure(true)
	if !s.TakeResumeDiscarded() {
		t.Fatal("invalidation did not flag the resume tuple as discarded")
	}
	// The flag is consumed, so the manager restarts its ladder exactly once.
	if s.TakeResumeDiscarded() {
		t.Fatal("TakeResumeDiscarded did not consume the flag")
	}
}

// Failures that invalidate nothing must not claim a discard.
func TestNoResumeDiscardedWithoutInvalidation(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	for i := 0; i < MaxResumeDialFailures*2; i++ {
		s.noteDialFailure(false)
	}
	if s.TakeResumeDiscarded() {
		t.Fatal("reported a discard for failures that did not use the resume URL")
	}

	s.noteDialSuccess()
	if s.TakeResumeDiscarded() {
		t.Fatal("reported a discard after a successful dial")
	}
}

// The incident log line is read under pressure; s.log already carries the shard,
// so noteDialFailure must not bind a second, duplicate "shard" field.
func TestNoteDialFailureDoesNotDuplicateShardField(t *testing.T) {
	src, err := os.ReadFile("resume.go")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(src, []byte(`slog.F("shard"`)) {
		t.Error(`resume.go binds slog.F("shard", ...) but s.log is already .With(shard) in NewSession`)
	}
}
