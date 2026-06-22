package gatewayws

import (
	"context"
	"hash/fnv"
	"os"
	"strconv"
	"time"

	"cdr.dev/slog"

	"github.com/tatsuworks/gateway/handler"
)

// LargeThreshold is Discord's large_threshold (members sent inline on
// GUILD_CREATE before the guild is considered "large").
const LargeThreshold = 250

// membersComplete reports whether the GUILD_CREATE payload already carries the
// full roster of a small guild: 0 < MemberCount <= threshold and we received at
// least MemberCount members.
func membersComplete(p *handler.EventPayload, threshold int) bool {
	return p.MemberCount > 0 &&
		p.MemberCount <= int64(threshold) &&
		p.ReceivedMembers >= int(p.MemberCount)
}

// backfillStalenessWindow is the base re-backfill cadence, from
// BACKFILL_STALENESS_HOURS (default 24h). It schedules the Phase 3b background
// sweep (which guilds are due for a refresh while connected); it is NOT part of
// the connect-time skip decision, which is no-expiry.
func backfillStalenessWindow() time.Duration {
	if v, err := strconv.Atoi(os.Getenv("BACKFILL_STALENESS_HOURS")); err == nil && v > 0 {
		return time.Duration(v) * time.Hour
	}
	return 24 * time.Hour
}

// jitterFactor maps a guild ID deterministically into [0.75, 1.25). The Phase
// 3b sweep multiplies the staleness window by it so guilds backfilled together
// (e.g. a cold deploy) come due for refresh spread over a ~12h band instead of
// all at once. Not used by the (no-expiry) skip decision.
func jitterFactor(guildID int64) float64 {
	h := fnv.New32a()
	var b [8]byte
	for i := 0; i < 8; i++ {
		b[i] = byte(uint64(guildID) >> (8 * i))
	}
	_, _ = h.Write(b[:])
	return 0.75 + float64(h.Sum32()%1000)/1000.0*0.5
}

// isFresh reports whether a backfill completed within the guild's jittered
// staleness threshold. The Phase 3b sweep uses it to decide which connected
// guilds are due for a re-backfill; it is NOT part of the (no-expiry) skip.
func isFresh(guildID int64, backfilledAt time.Time, base time.Duration) bool {
	threshold := time.Duration(float64(base) * jitterFactor(guildID))
	return time.Since(backfilledAt) < threshold
}

// maybeRequestGuildMembers decides, per GUILD_CREATE, whether to skip RGM
// (fresh marker), reconcile a small guild straight from the payload roster, or
// fall back to RGM. Only the small-guild branch does DB work, and that at most
// once per guild per staleness window.
func (s *Session) maybeRequestGuildMembers(ctx context.Context, p *handler.EventPayload) {
	switch {
	case s.isBackfilled(p.GuildID):
		s.log.Debug(ctx, "skipping rgm: backfilled", slog.F("guild", p.GuildID))
	case membersComplete(p, LargeThreshold):
		if err := s.stateDB.ReconcileGuildMembers(ctx, p.GuildID, p.MemberIDs); err != nil {
			s.log.Error(ctx, "reconcile guild members", slog.Error(err), slog.F("guild", p.GuildID))
		}
	default:
		s.requestGuildMembers(p.GuildID)
	}
}

func (s *Session) isBackfilled(guildID int64) bool {
	_, ok := s.backfilled[guildID] // nil-map read is safe and returns false
	return ok
}
