package gatewayws

import (
	"context"
	"sync/atomic"

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
}

// noteDialFailure records a failed dial. On reaching MaxResumeDialFailures the
// resume tuple is cleared in memory and persisted cleared, so both the next
// dial and any later pod restart use the main gateway URL. This is the
// automatic trigger for what ForceIdentify already does by hand.
//
// A failure only earns a strike when both of these hold:
//
//   - usedResumeURL — the dial targeted the resume host. Failures against the
//     main gateway URL have no resume state to blame or discard.
//   - hostResponded — the host actually answered and refused the WebSocket
//     upgrade, which is what a retired edge does (the incident's 503).
//     websocket.Dial returns a nil *http.Response for every failure where no
//     server replied: DNS, TLS, a refused connection, our own egress being
//     down. Those say nothing about whether this edge still serves the session,
//     and counting them would let a single shared network outage lasting three
//     attempts clear otherwise-valid resume tuples across the whole fleet —
//     buying a re-IDENTIFY and a full re-backfill per shard, the expensive
//     recovery path, precisely when the fleet is already struggling.
//
// An unanswered dial is ignored rather than forgiving: it leaves the strikes
// already earned from the host itself intact, so a transport blip landing in
// the middle of a run of 503s cannot reset the escape and strand the shard.
//
// Read-loop goroutine only: it mutates seq/sessID/resumeURL, which are owned by
// that goroutine (see applyForceIdentify).
func (s *Session) noteDialFailure(usedResumeURL, hostResponded bool) {
	if !usedResumeURL || !hostResponded {
		return
	}

	s.resumeDialFailures++
	if s.resumeDialFailures < MaxResumeDialFailures {
		return
	}

	// s.log already carries name+shard from NewSession; do not re-bind them.
	s.log.Info(context.Background(), "resume gateway url unreachable, discarding resume state",
		slog.F("resume_gateway_url", s.resumeURL),
		slog.F("consecutive_dial_failures", s.resumeDialFailures))

	s.resumeDialFailures = 0
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
