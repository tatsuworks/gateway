package gatewayws

import (
	"testing"
	"time"
)

// The answered-refusal budget used to be count-only, and under the reconnect
// ladder this branch adds three strikes land at roughly t=0, t≈2s and t≈6s. So
// any shared failure lasting more than about seven seconds — a Discord-side
// deploy, a Cloudflare 5xx wave — discarded the resume tuple of every shard
// holding one, simultaneously.
//
// The asymmetry is what makes that unacceptable. A blip that shards simply ride
// out costs nothing; a fleet-wide discard costs a serialized re-IDENTIFY through
// the 16 shardID%16 identify buckets, each held for calcIdentifyWait past READY
// — minutes at best, the better part of an hour with the members intent — plus
// a full guild backfill per shard.
//
// So a discard now needs BOTH MaxResumeDialFailures refusals AND
// MinResumeDialFailureSpan elapsed since the first of them. These tests pin the
// span half; resume_dial_failure_test.go pins the count half.

// A shared outage is short. However many refusals it packs into that window,
// the tuple must survive it untouched and unpersisted.
func TestAShortSharedOutageCannotDiscardTheResumeTuple(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	// Far more refusals than the count threshold, all inside the span — which is
	// exactly what the ladder produces while a shared 5xx is in progress.
	start := time.Now()
	for i := 0; i < MaxResumeDialFailures*5; i++ {
		s.noteDialFailureAt(start.Add(time.Duration(i)*time.Second), true, dialRefused)
	}

	if !s.shouldResume() {
		t.Fatalf("resume state discarded by %d refusals inside %v; a shared 5xx must not invalidate the fleet",
			MaxResumeDialFailures*5, MinResumeDialFailureSpan)
	}
	if db.calls != 0 {
		t.Fatalf("persisted a cleared tuple during a short shared outage (calls=%d), want 0", db.calls)
	}
}

// The point of the span is to delay the discard, not to prevent it: an edge that
// is genuinely retired keeps refusing, and the shard must still escape.
func TestAPersistentlyRefusingEdgeIsStillEscaped(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	start := time.Now()
	s.noteDialFailureAt(start, true, dialRefused)
	s.noteDialFailureAt(start.Add(MinResumeDialFailureSpan-time.Second), true, dialRefused)

	if !s.shouldResume() {
		t.Fatal("discarded one second before the span elapsed")
	}

	s.noteDialFailureAt(start.Add(MinResumeDialFailureSpan), true, dialRefused)

	if s.shouldResume() {
		t.Fatal("resume state survived a host that refused every dial for the full span")
	}
	if s.resumeURL != "" {
		t.Fatalf("resumeURL = %q, want cleared", s.resumeURL)
	}
	if db.calls != 1 {
		t.Fatalf("persisted %d times, want exactly 1 — a pod restart must not reload the dead tuple", db.calls)
	}
	if db.resumeURL != "" || db.sess != "" || db.seq != 0 {
		t.Fatalf("persisted tuple = (seq=%d sess=%q url=%q), want all cleared", db.seq, db.sess, db.resumeURL)
	}
}

// Both halves are required, not either. Two refusals spread over an hour are not
// the signature of a retired edge — they are two unlucky dials — so the count
// still has to be met.
func TestSpanAloneWithoutEnoughRefusalsDoesNotDiscard(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	start := time.Now()
	for i := 0; i < MaxResumeDialFailures-1; i++ {
		s.noteDialFailureAt(start.Add(time.Duration(i)*MinResumeDialFailureSpan), true, dialRefused)
	}

	if !s.shouldResume() {
		t.Fatalf("discarded after only %d refusals; the count threshold is %d",
			MaxResumeDialFailures-1, MaxResumeDialFailures)
	}
}

// The span must start at the first REFUSAL, not at the first failure of any
// kind. Otherwise a shared outage that begins as unanswered dials (DNS, egress,
// a Cloudflare edge dropping connections) silently pre-ages the refusal budget,
// and the moment the far end starts answering 5xx the fleet discards in the same
// seven seconds this finding is about.
func TestRefusalSpanIgnoresEarlierUnansweredFailures(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	start := time.Now()
	// A long run of silence first.
	for i := 0; i < 10; i++ {
		s.noteDialFailureAt(start.Add(time.Duration(i)*time.Minute), true, dialUnanswered)
	}
	// Then the shared failure starts answering, and the ladder lands three
	// refusals within seconds.
	refusalsBegin := start.Add(10 * time.Minute)
	for i := 0; i < MaxResumeDialFailures; i++ {
		s.noteDialFailureAt(refusalsBegin.Add(time.Duration(i)*2*time.Second), true, dialRefused)
	}

	if !s.shouldResume() {
		t.Fatal("earlier silence pre-aged the refusal span; a shared outage can still wipe the fleet in seconds")
	}
}

// A successful dial means the host is serving us again, so everything it was
// suspected of is void — including the elapsed span. Otherwise refusals from
// hours ago combine with three fresh ones to discard instantly.
func TestSuccessfulDialRestartsTheRefusalSpan(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	start := time.Now()
	s.noteDialFailureAt(start, true, dialRefused)
	s.noteDialFailureAt(start.Add(MinResumeDialFailureSpan-time.Second), true, dialRefused)

	s.noteDialSuccess()

	later := start.Add(MinResumeDialFailureSpan + time.Hour)
	for i := 0; i < MaxResumeDialFailures; i++ {
		s.noteDialFailureAt(later.Add(time.Duration(i)*2*time.Second), true, dialRefused)
	}

	if !s.shouldResume() {
		t.Fatal("a successful dial did not restart the refusal span")
	}
}

// 429 is never "this edge retired" — it is "come back later", and it is exactly
// what a mass re-IDENTIFY draws. Letting it spend the refusal budget closes a
// feedback loop: discards cause an identify burst, the burst draws 429s, the
// 429s cause more discards. It also cannot be answered by discarding, since
// IDENTIFY is the rate-limited operation and a shard holding a resume tuple
// skips the identify lock entirely.
func TestThrottledDialsNeverSpendTheRefusalBudget(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	start := time.Now()
	for i := 0; i < 100; i++ {
		s.noteDialFailureAt(start.Add(time.Duration(i)*time.Minute), true, dialThrottled)
	}

	if !s.shouldResume() {
		t.Fatal("429s discarded the resume tuple; being rate limited says nothing about the edge")
	}
	if db.calls != 0 {
		t.Fatalf("persisted a cleared tuple after 429s only (calls=%d), want 0", db.calls)
	}
}

// Ignored, not forgiving — the same discipline unanswered dials already follow.
// A 429 landing mid-run of 5xx must not reset the escape and strand the shard.
func TestThrottledDialsDoNotResetEarnedRefusals(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	start := time.Now()
	for i := 0; i < MaxResumeDialFailures-1; i++ {
		s.noteDialFailureAt(start.Add(time.Duration(i)*time.Second), true, dialRefused)
	}
	s.noteDialFailureAt(start.Add(time.Minute), true, dialThrottled)
	s.noteDialFailureAt(start.Add(MinResumeDialFailureSpan), true, dialRefused)

	if s.shouldResume() {
		t.Fatal("a 429 reset strikes already earned from the host itself")
	}
}

// A 429 is an ANSWER, so it must not feed the never-answered budget either —
// that one exists for a host that has vanished, and its ERROR-level discard line
// would misreport an ordinary rate limit as a shard down for an hour.
func TestThrottledDialsDoNotFeedTheNoResponseBudget(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	start := time.Now()
	s.noteDialFailureAt(start, true, dialThrottled)
	s.noteDialFailureAt(start.Add(MaxResumeNoResponseDuration+time.Hour), true, dialThrottled)

	if !s.shouldResume() {
		t.Fatal("429s aged into the never-answered budget; the host was answering the whole time")
	}
}

// The two thresholds encode "an answered refusal is stronger evidence than
// silence". If a retune ever inverted that the code would still compile and the
// rest of the suite would still pass, so pin the ordering itself.
func TestRefusalSpanStaysWellInsideTheSilenceBudget(t *testing.T) {
	if MinResumeDialFailureSpan >= MaxResumeNoResponseDuration {
		t.Fatalf("MinResumeDialFailureSpan (%v) >= MaxResumeNoResponseDuration (%v): an answered refusal must escape sooner than silence, not later",
			MinResumeDialFailureSpan, MaxResumeNoResponseDuration)
	}
}
