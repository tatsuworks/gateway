package gatewayws

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"cdr.dev/slog"
)

// MaxResumeDialFailures is how many consecutive dial-stage refusals from the
// persisted resume_gateway_url are needed before the resume tuple is thrown away
// and the shard falls back to the main gateway URL. It is one of two conditions:
// see MinResumeDialFailureSpan for the other.
//
// A resume URL points at one specific Discord edge. When that edge stops
// serving the shard the failure lands on the WebSocket *handshake* (503) —
// before IDENTIFY or RESUME is sent — so Discord never gets the chance to tell
// us the session is stale via INVALID_SESSION, and nothing else in the connect
// path ever demotes the resume URL. Without this counter the shard redials the
// same dead host once per retry forever, and because the tuple is persisted in
// the shards table a pod restart reloads it and resumes the identical loop.
//
// Kept small: a resume is only useful for a short window anyway, so the cost of
// discarding one too eagerly (a full IDENTIFY) is much lower than the cost of
// holding a dead one (a permanently down shard).
const MaxResumeDialFailures = 3

// MinResumeDialFailureSpan is how long a run of refusals must have been going
// on before it may discard the tuple. Both this and MaxResumeDialFailures must
// be satisfied.
//
// The count alone is not a safe trigger, because the reconnect ladder decides
// how fast it is spent: at reconnectBackoffBase the three strikes land at
// roughly t=0, t≈2s and t≈6s. That made any shared failure lasting more than
// about seven seconds — one bad Cloudflare minute in front of every edge at
// once, or a 429 wave — enough to discard the tuple of every shard holding one,
// simultaneously.
//
// The asymmetry is what makes that unacceptable rather than merely untidy. A
// blip the shards ride out costs nothing: they redial and RESUME. A fleet-wide
// discard costs a serialized re-IDENTIFY through the 16 shardID%16 identify
// buckets, each held for calcIdentifyWait past READY, plus a full guild backfill
// per shard — minutes at best, the better part of an hour with the members
// intent — and that identify burst can itself draw the 429s that discard more
// tuples.
//
// On the number. It is deliberately NOT sized to outlast a shared outage: how
// long an all-edge 5xx can last is not derivable from this repo, and a floor
// long enough to cover the worst case would delay escape from a genuinely
// retired edge by that same worst case. It is sized instead to the window in
// which a RESUME would still have been honoured, which is the only interval
// where discarding costs anything real:
//
//   - Inside it, the tuple still works, so a discard destroys a cheap recovery
//     and buys a gated re-IDENTIFY. This must not happen for a transient.
//   - Beyond it, Discord answers the RESUME with INVALID_SESSION, which clears
//     the tuple anyway (see the op-9 branch in handleInternalEvent) — so
//     discarding pre-empts a re-IDENTIFY that was already coming rather than
//     causing one, and the floor buys nothing further.
//
// Discord does not document that window; it is short, minutes rather than
// hours, so this sits at the upper end of plausible. Like
// MaxResumeNoResponseDuration the figure is reasoned rather than measured, and
// the error is again one-sided in the direction that matters: too long merely
// delays an escape that before this branch never happened at all.
//
// The cost, stated plainly: escape from a retired edge goes from ~8s (measured
// on staging) to ~3min. Against never, which is what the incident actually did.
//
// One consequence worth knowing during an incident: this clock lives in memory,
// like the counter it guards, so a pod restart restarts it. Three seconds of
// strikes always survived any pod that lived longer than three seconds; a
// three-minute span does not survive a pod crash-looping faster than that. A
// gateway pod in that state is a larger problem than this, and TATSU-2516 is
// the net for it.
const MinResumeDialFailureSpan = 3 * time.Minute

// MaxResumeNoResponseDuration is how long the resume host may go without
// answering a dial at all — DNS, TLS, a refused connection, our own egress being
// down — before the tuple is discarded anyway.
//
// An edge that is *decommissioned* rather than retired-but-answering stops
// resolving instead of serving a 503, so requiring an answered rejection would
// leave that shard trapped exactly as in the incident: resumeURL is only ever
// cleared by INVALID_SESSION (which needs a connection we cannot get), a manual
// ForceIdentify, or this file, so nothing would recover it but an operator.
//
// The asymmetry with the refusal budget above is the whole point. An answered
// rejection is the host telling us it will not serve this session — strong,
// host-specific evidence, so a few of them over a few minutes is enough. An
// unanswered dial is ambiguous: one dead edge and a fleet-wide egress outage
// look identical from here. So this budget must not fire while the tuple is
// merely suspicious, only once holding it has become pointless.
//
// An hour is chosen for safety, not detection speed. A Discord session does not
// survive that long disconnected: the RESUME would draw INVALID_SESSION, which
// clears the tuple anyway, so firing here pre-empts a re-IDENTIFY that was
// already coming rather than causing one. And escaping a genuinely dead edge in
// an hour is indistinguishable from escaping it in fifteen minutes when the
// alternative is never. Note the retention figure is reasoned, not measured —
// but the error is one-sided: too long merely delays an escape that previously
// never happened, while too short is what mass-invalidates during a shared
// outage.
//
// This is deliberately a DURATION rather than a failure count. Elapsed time is
// what actually makes discarding safe; a count only approximates it through
// whatever the reconnect ladder happens to be, so retuning the ladder would
// silently change what the threshold means while its justification still read
// the same. MinResumeDialFailureSpan exists because the refusal budget had that
// same defect.
//
// Worst case is bounded by more than the threshold: a shard holding a resume
// tuple skips the identify lock, so discarding moves it back into the gated
// path, and initEtcd buckets that lock by shardID%16. A fleet-wide
// re-IDENTIFY therefore drains serially through 16 buckets rather than
// bursting at Discord.
const MaxResumeNoResponseDuration = time.Hour

// dialFailureKind is what the far end said when a dial against the resume host
// failed. The three carry very different evidence about that host, so they are
// classified once, at the dial site, and drive different budgets.
type dialFailureKind int

const (
	// dialUnanswered: nothing replied. websocket.Dial returns a nil
	// *http.Response for every failure where HTTPClient.Do itself failed — DNS,
	// TLS, a refused connection, our own egress being down, our own per-dial
	// deadline. One dead edge and a fleet-wide outage are indistinguishable from
	// here.
	dialUnanswered dialFailureKind = iota
	// dialRefused: the host answered and refused the upgrade — a non-101 status
	// erroring out of verifyServerResponse with the response non-nil. This is
	// the incident's 503 and what a retired edge does.
	dialRefused
	// dialThrottled: the host answered 429. An answer, but the one answer that
	// is never host-specific. See noteDialFailureAt.
	dialThrottled
)

func (k dialFailureKind) String() string {
	switch k {
	case dialRefused:
		return "refused"
	case dialThrottled:
		return "throttled"
	default:
		return "unanswered"
	}
}

// classifyDialFailure reads the response websocket.Dial returned alongside its
// error. It is the single discriminator the whole resume-invalidation policy
// rests on, so it lives here next to the policy rather than inline at the call
// site, and is tested against a real answered rejection (see
// dial_classification_test.go) rather than only through hand-chosen values.
func classifyDialFailure(resp *http.Response) dialFailureKind {
	switch {
	case resp == nil:
		return dialUnanswered
	case resp.StatusCode == http.StatusTooManyRequests:
		return dialThrottled
	default:
		return dialRefused
	}
}

// usingResumeURL reports whether the next dial will target the persisted resume
// URL rather than the main gateway URL. It is the same condition GatewayURL
// branches on, so a dial failure can be attributed to the resume host.
func (s *Session) usingResumeURL() bool {
	return s.resumeURL != "" && s.sessID != ""
}

// noteDialSuccess clears every suspicion held against the resume host: the
// refusal count and both elapsed-time clocks. The host is serving us again, so
// nothing earned before now says anything about it — and only failures in a row
// mean anything, or isolated blips would accumulate across hours of healthy
// reconnects into a spurious invalidation.
//
// Read-loop goroutine only (it is called from conn.run).
func (s *Session) noteDialSuccess() {
	s.resumeDialFailures = 0
	s.resumeDialFailuresSince = time.Time{}
	s.resumeNoResponseSince = time.Time{}
}

// noteDialFailure records a failed dial. When a budget is exhausted the resume
// tuple is cleared in memory and persisted cleared, so both the next dial and
// any later pod restart use the main gateway URL. This is the automatic trigger
// for what ForceIdentify already does by hand.
//
// Only failures against the resume host count at all: usedResumeURL false means
// the dial targeted the main gateway URL, which has no resume state to blame or
// discard.
//
// kind splits the rest into two independent budgets and one exemption:
//
//   - dialRefused — the host replied and refused the upgrade, which is what a
//     retired edge does. Host-specific, so the budget is small: it needs
//     MaxResumeDialFailures refusals AND MinResumeDialFailureSpan elapsed since
//     the first of them. Both, because the count alone is spent in seconds at
//     the base of the reconnect ladder, which would let a brief shared 5xx
//     discard the tuple of every shard at once.
//   - dialUnanswered — nothing replied, so the failure is ambiguous between one
//     dead edge and a fleet-wide outage. Bounded by elapsed time only
//     (MaxResumeNoResponseDuration): long enough that the tuple is worthless by
//     the time it fires, but still finite, so a decommissioned edge that stopped
//     resolving cannot trap the shard forever.
//   - dialThrottled — the host answered 429. Never counted, and never a discard
//     on its own budget. 429 does not mean "this edge retired", it means "come
//     back later", and it is precisely what a mass re-IDENTIFY draws: letting it
//     spend the refusal budget closes a feedback loop where discards cause an
//     identify burst, the burst draws 429s, and those 429s discard more tuples.
//     It cannot be answered by discarding either — IDENTIFY is the rate-limited
//     operation, and a shard holding a resume tuple skips the identify lock
//     entirely (see the shouldResume branch in conn.run), so discarding under a
//     429 strictly worsens the shard's odds. Being unbounded here is therefore
//     correct rather than a gap: a host that answers at all is not the
//     vanished-edge case MaxResumeNoResponseDuration exists for.
//
// The budgets are independent and none of them clears another, so a host that
// alternates between refusing the upgrade and not answering still escapes, and
// neither a transport blip nor a 429 landing mid-run of 503s can reset an escape
// already earned from the host itself. Only a successful dial clears anything.
//
// Read-loop goroutine only: it mutates seq/sessID/resumeURL, which are owned by
// that goroutine (see applyForceIdentify).
func (s *Session) noteDialFailure(usedResumeURL bool, kind dialFailureKind) {
	s.noteDialFailureAt(time.Now(), usedResumeURL, kind)
}

// noteDialFailureAt is noteDialFailure with the clock injected, so the two
// elapsed-time bounds can be tested without waiting out a real hour.
func (s *Session) noteDialFailureAt(now time.Time, usedResumeURL bool, kind dialFailureKind) {
	if !usedResumeURL {
		return
	}

	switch kind {
	case dialThrottled:
		// Ignored, not forgiving: it neither spends a budget nor clears one.
		return

	case dialRefused:
		s.resumeDialFailures++
		if s.resumeDialFailuresSince.IsZero() {
			// Stamped on the first REFUSAL, not on the first failure of any
			// kind: otherwise a shared outage that begins as unanswered dials
			// pre-ages this clock, and the moment the far end starts answering
			// 5xx the fleet discards inside the same few seconds the span is
			// here to prevent.
			s.resumeDialFailuresSince = now
		}
		refusedFor := now.Sub(s.resumeDialFailuresSince)
		if s.resumeDialFailures >= MaxResumeDialFailures && refusedFor >= MinResumeDialFailureSpan {
			// Warn, not Error: an edge refusing us for minutes on end is worth
			// surfacing, but it is also what an ordinary edge retirement looks
			// like, so it must not cry wolf.
			s.log.Warn(context.Background(), "resume gateway url unreachable, discarding resume state",
				slog.F("resume_gateway_url", s.resumeURL),
				slog.F("consecutive_dial_failures", s.resumeDialFailures),
				slog.F("refused_for", refusedFor))
			s.clearResumeState()
		}
		return

	default: // dialUnanswered
		if s.resumeNoResponseSince.IsZero() {
			s.resumeNoResponseSince = now
			return
		}
		if unreachableFor := now.Sub(s.resumeNoResponseSince); unreachableFor >= MaxResumeNoResponseDuration {
			// Error, and a distinct message from the answered case. Reaching
			// here means this shard has failed every dial for
			// MaxResumeNoResponseDuration with nothing on the other end — i.e.
			// it has been down for about an hour. That is an incident, and the
			// log level is the only thing that carries it anywhere: nothing
			// pages on this class of outage today (TATSU-2516). During an
			// incident it also matters whether the edge refused us or vanished,
			// since the two have very different blast radii.
			s.log.Error(context.Background(), "resume gateway url never answered, discarding resume state",
				slog.F("resume_gateway_url", s.resumeURL),
				slog.F("unreachable_for", unreachableFor))
			s.clearResumeState()
		}
	}
}

// clearResumeState throws the resume tuple away and persists it cleared, so both
// the next dial and any later pod restart use the main gateway URL. This is the
// automatic trigger for what ForceIdentify already does by hand.
//
// Read-loop goroutine only, same ownership as noteDialFailure.
func (s *Session) clearResumeState() {
	s.resumeDialFailures = 0
	s.resumeDialFailuresSince = time.Time{}
	s.resumeNoResponseSince = time.Time{}
	atomic.StoreInt32(&s.resumeDiscarded, 1)
	atomic.StoreInt64(&s.seq, 0)
	s.sessID = ""
	s.resumeURL = ""
	s.persistShardInfo()
}

// TakeResumeDiscarded reports whether the resume tuple was discarded since the
// last call, clearing the flag. The manager's reconnect loop uses it to restart
// its backoff ladder: once the tuple is gone the next dial targets the main
// gateway URL, a different host from the one those failures came from, so the
// delay they earned predicts nothing about it. Without this the escape pays for
// a wait it no longer needs -- measured on staging as 9.0s of a 15.8s recovery.
//
// Set on the read-loop goroutine, read on the manager goroutine, hence atomic.
func (s *Session) TakeResumeDiscarded() bool {
	return atomic.SwapInt32(&s.resumeDiscarded, 0) == 1
}
