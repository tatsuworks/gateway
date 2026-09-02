package gatewayws

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

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

// refuse drives n answered refusals against the resume host, spaced far enough
// apart that MinResumeDialFailureSpan is satisfied by the second one. Discarding
// needs a count AND an elapsed span; the span is pinned on its own in
// resume_shared_outage_test.go, so the tests below hold it satisfied and vary
// only the count.
func refuse(s *Session, n int) {
	start := time.Now()
	for i := 0; i < n; i++ {
		s.noteDialFailureAt(start.Add(time.Duration(i)*MinResumeDialFailureSpan), true, dialRefused)
	}
}

// Below the threshold the resume tuple is still the best guess (a single dial
// failure is far more likely to be a transient blip than a retired edge), so it
// must survive untouched and unpersisted.
func TestResumeDialFailuresBelowThresholdKeepResumeState(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	refuse(s, MaxResumeDialFailures-1)

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

	refuse(s, MaxResumeDialFailures)

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

	refuse(s, MaxResumeDialFailures-1)
	s.noteDialSuccess()
	refuse(s, MaxResumeDialFailures-1)

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

	start := time.Now()
	for i := 0; i < MaxResumeDialFailures*3; i++ {
		s.noteDialFailureAt(start.Add(time.Duration(i)*MinResumeDialFailureSpan), false, dialRefused)
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

	refuse(s, MaxResumeDialFailures)
	if db.calls != 1 {
		t.Fatalf("SetShardInfo calls = %d after invalidation, want 1", db.calls)
	}

	// The tuple is empty now, so subsequent dials use the main URL and report
	// usedResumeURL=false; nothing further should be persisted.
	start := time.Now()
	for i := 0; i < MaxResumeDialFailures*2; i++ {
		s.noteDialFailureAt(start.Add(time.Duration(i)*MinResumeDialFailureSpan), false, dialRefused)
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

	start := time.Now()
	for i := 0; i < MaxResumeDialFailures-1; i++ {
		s.noteDialFailureAt(start.Add(time.Duration(i)*MinResumeDialFailureSpan), true, dialRefused)
	}
	if s.TakeResumeDiscarded() {
		t.Fatal("reported a discard below the failure threshold")
	}

	s.noteDialFailureAt(start.Add(MaxResumeDialFailures*MinResumeDialFailureSpan), true, dialRefused)
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

	start := time.Now()
	for i := 0; i < MaxResumeDialFailures*2; i++ {
		s.noteDialFailureAt(start.Add(time.Duration(i)*MinResumeDialFailureSpan), false, dialRefused)
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

// A dial can fail without the resume host ever answering — DNS, TLS, a refused
// connection, or our own egress being down. Those say nothing about whether the
// edge still serves this session, and treating them like a refusal lets one
// shared network outage clear valid resume tuples across the whole fleet,
// forcing an IDENTIFY and a full re-backfill per shard once connectivity
// returns. Only a host that answered and refused the upgrade (the incident's
// 503) spends a strike.
func TestDialFailuresWithNoResponseDoNotInvalidate(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	// Deliberately a large NUMBER of failures inside a short SPAN: what matters
	// is how long the host has been unreachable, not how many times we asked.
	start := time.Now()
	for i := 0; i < 200; i++ {
		s.noteDialFailureAt(start.Add(time.Duration(i)*time.Second), true, dialUnanswered)
	}

	if !s.shouldResume() {
		t.Fatal("resume state discarded by 200 unanswered dials spanning only ~3 minutes")
	}
	if s.resumeURL == "" {
		t.Fatal("resumeURL cleared by transport failures with no HTTP response")
	}
	if db.calls != 0 {
		t.Fatalf("persisted shard info (calls=%d), want 0", db.calls)
	}
	if s.TakeResumeDiscarded() {
		t.Fatal("reported a discard for failures the host never answered")
	}
}

// An edge that is decommissioned rather than retired-but-answering stops
// resolving instead of serving a 503. Requiring an answered rejection would
// leave such a shard trapped exactly as in the incident, recoverable only by a
// manual gwForceIdentify — so an unanswered dial still escapes, once the host
// has been unreachable long enough that the tuple is worthless anyway.
//
// The bound is a DURATION, not a count. What makes discarding safe is elapsed
// time (past it, a RESUME draws INVALID_SESSION regardless); a strike count only
// approximates that through whatever the reconnect ladder happens to be, and
// would silently mean something different if the ladder were ever retuned.
func TestNoResponseInvalidatesOnlyAfterTheFullWindow(t *testing.T) {
	if MaxResumeNoResponseDuration < 10*time.Minute {
		t.Fatalf("MaxResumeNoResponseDuration = %v: too short to distinguish a dead edge from a shared outage",
			MaxResumeNoResponseDuration)
	}

	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)
	// The discard here logs at ERROR on purpose (see TestDiscardLogLevelsCarryTheAlert);
	// slogtest fatals the test on an Error line unless told otherwise.
	s.log = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	start := time.Now()
	s.noteDialFailureAt(start, true, dialUnanswered)
	s.noteDialFailureAt(start.Add(MaxResumeNoResponseDuration-time.Second), true, dialUnanswered)

	if !s.shouldResume() {
		t.Fatal("resume state discarded one second inside the no-response window")
	}
	if db.calls != 0 {
		t.Fatalf("persisted shard info inside the window (calls=%d), want 0", db.calls)
	}

	s.noteDialFailureAt(start.Add(MaxResumeNoResponseDuration), true, dialUnanswered)
	if s.shouldResume() {
		t.Fatal("session still resumable after a full window of unanswered dials")
	}
	if s.sessID != "" || s.resumeURL != "" {
		t.Fatalf("sessID=%q resumeURL=%q, want both empty", s.sessID, s.resumeURL)
	}
	if db.calls != 1 {
		t.Fatalf("SetShardInfo calls = %d, want exactly 1 (persist the cleared tuple once)", db.calls)
	}
	if !s.TakeResumeDiscarded() {
		t.Fatal("no-response invalidation did not flag the resume tuple as discarded")
	}
}

// The window measures one unbroken run of unreachability, not absolute age. A
// dial that connects means the host is fine, so a later failure starts over
// rather than inheriting credit from an outage that has since cleared.
func TestSuccessfulDialRestartsTheNoResponseWindow(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	start := time.Now()
	s.noteDialFailureAt(start, true, dialUnanswered)
	s.noteDialSuccess()

	// Far past the original window, but the first failure of a fresh run.
	s.noteDialFailureAt(start.Add(MaxResumeNoResponseDuration+time.Hour), true, dialUnanswered)

	if !s.shouldResume() {
		t.Fatal("resume state discarded: a successful dial did not restart the no-response window")
	}
	if db.calls != 0 {
		t.Fatalf("persisted shard info (calls=%d), want 0", db.calls)
	}
}

// An unanswered dial is ignored, not forgiving: it leaves the strikes already
// earned from the host itself intact, so a transport blip landing in the middle
// of a run of 503s cannot reset the escape and strand the shard.
func TestNoResponseFailuresDoNotResetEarnedStrikes(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	// Spaced so the refusal span is satisfied; the point under test is that the
	// unanswered dial in the middle does not undo the strikes around it.
	start := time.Now()
	for i := 0; i < MaxResumeDialFailures-1; i++ {
		s.noteDialFailureAt(start.Add(time.Duration(i)*MinResumeDialFailureSpan), true, dialRefused)
	}
	s.noteDialFailureAt(start.Add(MaxResumeDialFailures*MinResumeDialFailureSpan), true, dialUnanswered)
	if !s.shouldResume() {
		t.Fatal("resume state discarded early by an unanswered dial")
	}

	s.noteDialFailureAt(start.Add((MaxResumeDialFailures+1)*MinResumeDialFailureSpan), true, dialRefused)
	if s.shouldResume() {
		t.Fatal("session still resumable: an unanswered dial reset the earned strikes")
	}
	if !s.TakeResumeDiscarded() {
		t.Fatal("invalidation did not flag the resume tuple as discarded")
	}
}

// The two budgets are independent evidence and neither clears the other, so a
// host that alternates between refusing the upgrade and not answering at all
// still escapes rather than sitting below both thresholds forever.
func TestAnsweredAndUnansweredBudgetsAreIndependent(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	start := time.Now()
	for i := 0; i < MaxResumeDialFailures-1; i++ {
		at := start.Add(time.Duration(i) * MinResumeDialFailureSpan)
		s.noteDialFailureAt(at, true, dialRefused)
		s.noteDialFailureAt(at, true, dialUnanswered)
	}
	if !s.shouldResume() {
		t.Fatal("resume state discarded below both thresholds")
	}

	// The answered budget is the one that fills first here: its span is minutes,
	// the unanswered one's is an hour.
	s.noteDialFailureAt(start.Add(MaxResumeDialFailures*MinResumeDialFailureSpan), true, dialRefused)
	if s.shouldResume() {
		t.Fatal("unanswered dials reset the answered strike count")
	}
}

// captureSink records log entries so a test can assert the level a line is
// emitted at, not just that it happened.
type captureSink struct{ entries []slog.SinkEntry }

func (c *captureSink) LogEntry(_ context.Context, e slog.SinkEntry) { c.entries = append(c.entries, e) }
func (c *captureSink) Sync()                                        {}

func (c *captureSink) find(msg string) (slog.SinkEntry, bool) {
	for _, e := range c.entries {
		if strings.Contains(e.Message, msg) {
			return e, true
		}
	}
	return slog.SinkEntry{}, false
}

// The severity of the two discard lines is load-bearing, not cosmetic. Nothing
// pages on this class of outage today (TATSU-2516), so until a metrics surface
// exists the log level is the only thing that carries the event anywhere — a
// silent downgrade to Info would disable an alert without failing anything else.
//
//   - Never-answered: reaching it means the shard failed every dial for
//     MaxResumeNoResponseDuration with nothing on the other end, i.e. it has been
//     down for about an hour. That is an incident → ERROR.
//   - Answered-and-refused: worth surfacing, but also what an ordinary Discord
//     edge retirement looks like → WARN, not ERROR, so it does not cry wolf.
func TestDiscardLogLevelsCarryTheAlert(t *testing.T) {
	t.Run("never answered is ERROR", func(t *testing.T) {
		sink := &captureSink{}
		db := &shardInfoRecorder{}
		s, _ := newResumableSession(t, db)
		s.log = slog.Make(sink)

		start := time.Now()
		s.noteDialFailureAt(start, true, dialUnanswered)
		s.noteDialFailureAt(start.Add(MaxResumeNoResponseDuration), true, dialUnanswered)

		e, ok := sink.find("never answered")
		if !ok {
			t.Fatal("no discard line logged for the never-answered path")
		}
		if e.Level != slog.LevelError {
			t.Fatalf("never-answered discard logged at %v, want ERROR — nothing else surfaces this outage", e.Level)
		}
	})

	t.Run("answered and refused is WARN", func(t *testing.T) {
		sink := &captureSink{}
		db := &shardInfoRecorder{}
		s, _ := newResumableSession(t, db)
		s.log = slog.Make(sink)

		refuse(s, MaxResumeDialFailures)

		e, ok := sink.find("unreachable, discarding")
		if !ok {
			t.Fatal("no discard line logged for the answered path")
		}
		if e.Level != slog.LevelWarn {
			t.Fatalf("answered discard logged at %v, want WARN (routine edge retirement must not page)", e.Level)
		}
	})
}
