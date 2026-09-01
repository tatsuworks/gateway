package gatewayws

import (
	"context"
	"sync/atomic"
	"time"

	"cdr.dev/slog"
)

// MaxResumeDialFailures is how many consecutive dial-stage failures against the
// persisted resume_gateway_url are tolerated before the resume tuple is thrown
// away and the shard falls back to the main gateway URL.
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
// The asymmetry with MaxResumeDialFailures is the whole point. An answered
// rejection is the host telling us it will not serve this session — strong,
// host-specific evidence, so 3 is enough. An unanswered dial is ambiguous: one
// dead edge and a fleet-wide egress outage look identical from here. So this
// budget must not fire while the tuple is merely suspicious, only once holding
// it has become pointless.
//
// An hour is chosen for safety, not detection speed. A Discord session does not
// survive that long disconnected: the RESUME would draw INVALID_SESSION, which
// clears the tuple anyway (see the op-9 branch in handleInternalEvent), so
// firing here pre-empts a re-IDENTIFY that was already coming rather than
// causing one. And escaping a genuinely dead edge in an hour is
// indistinguishable from escaping it in fifteen minutes when the alternative is
// never. Note the retention figure is reasoned, not measured — but the error is
// one-sided: too long merely delays an escape that previously never happened,
// while too short is what mass-invalidates during a shared outage.
//
// This is deliberately a DURATION rather than a failure count. Elapsed time is
// what actually makes discarding safe; a count only approximates it through
// whatever the reconnect ladder happens to be, so retuning the ladder would
// silently change what the threshold means while its justification still read
// the same.
//
// Worst case is bounded by more than the threshold: a shard holding a resume
// tuple skips the identify lock, so discarding moves it back into the gated
// path, and initEtcd buckets that lock by shardID%16. A fleet-wide
// re-IDENTIFY therefore drains serially through 16 buckets rather than
// bursting at Discord.
const MaxResumeNoResponseDuration = time.Hour

// usingResumeURL reports whether the next dial will target the persisted resume
// URL rather than the main gateway URL. It is the same condition GatewayURL
// branches on, so a dial failure can be attributed to the resume host.
func (s *Session) usingResumeURL() bool {
	return s.resumeURL != "" && s.sessID != ""
}

// noteDialSuccess clears the consecutive dial-failure count. Only failures in a
// row mean anything: an isolated blip must not accumulate across hours of
// healthy reconnects into a spurious invalidation.
//
// Read-loop goroutine only (it is called from conn.run).
func (s *Session) noteDialSuccess() {
	s.resumeDialFailures = 0
	s.resumeNoResponseSince = time.Time{}
}

// noteDialFailure records a failed dial. On reaching MaxResumeDialFailures the
// resume tuple is cleared in memory and persisted cleared, so both the next
// dial and any later pod restart use the main gateway URL. This is the
// automatic trigger for what ForceIdentify already does by hand.
//
// Only failures against the resume host count at all: usedResumeURL false means
// the dial targeted the main gateway URL, which has no resume state to blame or
// discard.
//
// hostResponded splits the rest into two independent budgets, because the two
// carry very different evidence:
//
//   - Answered (hostResponded, i.e. websocket.Dial returned a non-nil
//     *http.Response): the host replied and refused the upgrade, which is what
//     a retired edge does — the incident's 503. Host-specific and conclusive,
//     so MaxResumeDialFailures is small.
//   - Unanswered: websocket.Dial returns a nil response for every failure where
//     nothing replied — DNS, TLS, a refused connection, our own egress being
//     down. One dead edge and a fleet-wide outage are indistinguishable from
//     here, so spending a strike on these would let a shared outage lasting
//     three attempts clear valid resume tuples across the whole fleet. They are
//     bounded by elapsed time instead (MaxResumeNoResponseDuration): long
//     enough that the tuple is worthless by the time it fires, but still finite,
//     so a decommissioned edge that stopped resolving cannot trap the shard
//     forever.
//
// The two bounds are independent and neither clears the other, so a host that
// alternates between refusing the upgrade and not answering still escapes, and
// a transport blip landing mid-run of 503s cannot reset that escape. Only a
// successful dial clears both.
//
// Read-loop goroutine only: it mutates seq/sessID/resumeURL, which are owned by
// that goroutine (see applyForceIdentify).
func (s *Session) noteDialFailure(usedResumeURL, hostResponded bool) {
	s.noteDialFailureAt(time.Now(), usedResumeURL, hostResponded)
}

// noteDialFailureAt is noteDialFailure with the clock injected, so the
// no-response window can be tested without waiting out a real hour.
func (s *Session) noteDialFailureAt(now time.Time, usedResumeURL, hostResponded bool) {
	if !usedResumeURL {
		return
	}

	if hostResponded {
		s.resumeDialFailures++
		if s.resumeDialFailures >= MaxResumeDialFailures {
			// Warn, not Info: an edge refusing us three times running is worth
			// surfacing, but it is also what an ordinary edge retirement looks
			// like, so it is not on its own an incident.
			s.log.Warn(context.Background(), "resume gateway url unreachable, discarding resume state",
				slog.F("resume_gateway_url", s.resumeURL),
				slog.F("consecutive_dial_failures", s.resumeDialFailures))
			s.clearResumeState()
		}
		return
	}

	if s.resumeNoResponseSince.IsZero() {
		s.resumeNoResponseSince = now
		return
	}
	if unreachableFor := now.Sub(s.resumeNoResponseSince); unreachableFor >= MaxResumeNoResponseDuration {
		// Error, and a distinct message from the answered case. Reaching here
		// means this shard has failed every dial for MaxResumeNoResponseDuration
		// with nothing on the other end — i.e. it has been down for about an
		// hour. That is an incident, and the log level is the only thing that
		// carries it anywhere: nothing pages on this class of outage today
		// (TATSU-2516). During an incident it also matters whether the edge
		// refused us or vanished, since the two have very different blast radii.
		s.log.Error(context.Background(), "resume gateway url never answered, discarding resume state",
			slog.F("resume_gateway_url", s.resumeURL),
			slog.F("unreachable_for", unreachableFor))
		s.clearResumeState()
	}
}

// clearResumeState throws the resume tuple away and persists it cleared, so both
// the next dial and any later pod restart use the main gateway URL. This is the
// automatic trigger for what ForceIdentify already does by hand.
//
// Read-loop goroutine only, same ownership as noteDialFailure.
func (s *Session) clearResumeState() {
	s.resumeDialFailures = 0
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
