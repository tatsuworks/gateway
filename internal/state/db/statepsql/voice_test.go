package statepsql

import (
	"context"
	"math/rand"
	"os"
	"testing"
	"time"

	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/tatsuworks/gateway/internal/state"
)

// newVoiceTestDB connects to STATE_TEST_PSQL (default: the same local database
// the other statepsql tests use) and skips when it is unavailable. The
// database needs init.sql and upgrades/0001_voice_attendance.sql applied.
func newVoiceTestDB(t *testing.T) *db {
	t.Helper()
	addr := os.Getenv("STATE_TEST_PSQL")
	if addr == "" {
		addr = "postgresql://tatsu@localhost/state?sslmode=disable"
	}
	d, err := NewDB(context.Background(), addr, sloghuman.Make(os.Stderr))
	if err != nil {
		t.Skipf("skipping: postgres not available: %v", err)
	}
	return d.(*db)
}

func boolPtr(b bool) *bool { return &b }

// testShard picks a shard id unlikely to collide with concurrent test runs.
func testShard() int { return 100000 + rand.Intn(1_000_000) }

func mustSnapshot(t *testing.T, d *db, guild int64, after int64) *state.VoiceSnapshot {
	t.Helper()
	snap, err := d.GetGuildVoiceSnapshot(context.Background(), guild, after, 15*time.Second)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return snap
}

func baselineFor(guild int64, shard int, conn string, occupants ...state.VoiceOccupant) state.VoiceBaseline {
	return state.VoiceBaseline{
		GuildID:      guild,
		ShardID:      shard,
		ConnectionID: conn,
		ReceivedAt:   time.Now().UTC(),
		DiscordSeq:   7,
		Occupants:    occupants,
		Raws:         map[int64][]byte{},
	}
}

func TestVoiceSnapshotRequiresBaselineAndFreshHeartbeat(t *testing.T) {
	d := newVoiceTestDB(t)
	ctx := context.Background()
	guild, shard := rand.Int63(), testShard()

	if snap := mustSnapshot(t, d, guild, 0); snap.Ready || snap.NotReadyReason != voiceNotReadyNoBaseline || snap.ShardID != -1 {
		t.Fatalf("no baseline must be unready with no shard, got %+v", snap)
	}

	if err := d.ReplaceGuildVoiceStates(ctx, baselineFor(guild, shard, "conn-a")); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if snap := mustSnapshot(t, d, guild, 0); snap.Ready || snap.NotReadyReason != voiceNotReadyNoHeartbeat {
		t.Fatalf("a baseline without a heartbeat must be unready, got %+v", snap)
	}

	if _, err := d.HeartbeatVoiceShard(ctx, shard, "conn-a", true); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	snap := mustSnapshot(t, d, guild, 0)
	if !snap.Ready || !snap.Complete || len(snap.Occupants) != 0 {
		t.Fatalf("verified empty presence must be ready and complete with no occupants, got %+v", snap)
	}

	// A heartbeat older than the limit is stale, never ready.
	if _, err := d.sql.Exec(`UPDATE voice_shard_status SET heartbeat_at = now() - interval '20 seconds' WHERE shard_id = $1`, shard); err != nil {
		t.Fatalf("age heartbeat: %v", err)
	}
	if snap := mustSnapshot(t, d, guild, 0); snap.Ready || snap.NotReadyReason != voiceNotReadyStale {
		t.Fatalf("stale heartbeat must be unready, got %+v", snap)
	}

	// A shard that reports itself not ready (an unrecorded loss) is unready.
	if _, err := d.HeartbeatVoiceShard(ctx, shard, "conn-a", false); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if snap := mustSnapshot(t, d, guild, 0); snap.Ready || snap.NotReadyReason != voiceNotReadyShardNotReady {
		t.Fatalf("not-ready shard must be unready, got %+v", snap)
	}

	// A new connection invalidates the old connection's baseline until a fresh
	// GUILD_CREATE arrives on it.
	if _, err := d.HeartbeatVoiceShard(ctx, shard, "conn-b", true); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if snap := mustSnapshot(t, d, guild, 0); snap.Ready || snap.NotReadyReason != voiceNotReadyConnection {
		t.Fatalf("baseline from another connection must be unready, got %+v", snap)
	}
	if err := d.ReplaceGuildVoiceStates(ctx, baselineFor(guild, shard, "conn-b")); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if snap := mustSnapshot(t, d, guild, 0); !snap.Ready {
		t.Fatalf("fresh baseline on the current connection must be ready, got %+v", snap)
	}
}

func TestVoiceBaselineReplacesPresenceAndClassifies(t *testing.T) {
	d := newVoiceTestDB(t)
	ctx := context.Background()
	guild, shard := rand.Int63(), testShard()
	human, bot, gone := rand.Int63(), rand.Int63(), rand.Int63()
	channel := rand.Int63()

	if err := d.ApplyVoiceState(ctx, state.VoiceStateChange{GuildID: guild, UserID: gone, ChannelID: channel, IsBot: boolPtr(false), Raw: []byte(`{}`)}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := d.ReplaceGuildVoiceStates(ctx, baselineFor(guild, shard, "conn",
		state.VoiceOccupant{UserID: human, ChannelID: channel, IsBot: boolPtr(false)},
		state.VoiceOccupant{UserID: bot, ChannelID: channel, IsBot: boolPtr(true)},
	)); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if _, err := d.HeartbeatVoiceShard(ctx, shard, "conn", true); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	snap := mustSnapshot(t, d, guild, 0)
	if !snap.Ready || !snap.Complete || len(snap.Occupants) != 2 {
		t.Fatalf("baseline must replace earlier presence exactly, got %+v", snap)
	}
	byUser := map[int64]state.VoiceOccupant{}
	for _, o := range snap.Occupants {
		byUser[o.UserID] = o
	}
	if _, ok := byUser[gone]; ok {
		t.Fatalf("baseline must drop users it does not list")
	}
	if o := byUser[bot]; o.IsBot == nil || !*o.IsBot {
		t.Fatalf("bot must stay classified as bot, got %+v", o)
	}
	if o := byUser[human]; o.IsBot == nil || *o.IsBot || o.DiscordSeq != 7 {
		t.Fatalf("human must be classified with the baseline sequence, got %+v", o)
	}
}

func TestVoiceUnclassifiedOccupantIsIncompleteUntilMemberKnown(t *testing.T) {
	d := newVoiceTestDB(t)
	ctx := context.Background()
	guild, shard := rand.Int63(), testShard()
	user, channel := rand.Int63(), rand.Int63()

	if err := d.ReplaceGuildVoiceStates(ctx, baselineFor(guild, shard, "conn")); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if _, err := d.HeartbeatVoiceShard(ctx, shard, "conn", true); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if err := d.ApplyVoiceState(ctx, state.VoiceStateChange{GuildID: guild, UserID: user, ChannelID: channel, Raw: []byte(`{}`)}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	snap := mustSnapshot(t, d, guild, 0)
	if !snap.Ready || snap.Complete || snap.Occupants[0].IsBot != nil {
		t.Fatalf("an unknown account must leave the snapshot incomplete, got %+v", snap)
	}

	// A cached member with a user object and no bot flag is a human.
	if _, err := d.sql.Exec(`INSERT INTO members (guild_id, user_id, data) VALUES ($1, $2, '{"user":{"id":"1"}}')`, guild, user); err != nil {
		t.Fatalf("seed member: %v", err)
	}
	if err := d.ApplyVoiceState(ctx, state.VoiceStateChange{GuildID: guild, UserID: user, ChannelID: channel, Raw: []byte(`{}`)}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	snap = mustSnapshot(t, d, guild, 0)
	if !snap.Complete || snap.Occupants[0].IsBot == nil || *snap.Occupants[0].IsBot {
		t.Fatalf("cached member must classify the occupant as human, got %+v", snap)
	}
}

func TestVoiceApplyMovesLeavesAndKeepsArrivalOnMute(t *testing.T) {
	d := newVoiceTestDB(t)
	ctx := context.Background()
	guild, shard := rand.Int63(), testShard()
	user, movie, game := rand.Int63(), rand.Int63(), rand.Int63()
	if err := d.ReplaceGuildVoiceStates(ctx, baselineFor(guild, shard, "conn")); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if _, err := d.HeartbeatVoiceShard(ctx, shard, "conn", true); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	joined := time.Date(2026, 11, 10, 18, 0, 0, 0, time.UTC)
	apply := func(channel int64, at time.Time, seq int64) {
		t.Helper()
		if err := d.ApplyVoiceState(ctx, state.VoiceStateChange{
			GuildID: guild, UserID: user, ChannelID: channel, IsBot: boolPtr(false),
			ChangedAt: at, DiscordSeq: seq, Raw: []byte(`{}`),
		}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}
	apply(movie, joined, 1)
	apply(movie, joined.Add(5*time.Minute), 2) // mute: same channel
	snap := mustSnapshot(t, d, guild, 0)
	if got := snap.Occupants[0]; got.ChannelID != movie || !got.ChangedAt.Equal(joined) || got.DiscordSeq != 2 {
		t.Fatalf("same-channel update must keep arrival time, got %+v", got)
	}

	moved := joined.Add(15 * time.Minute)
	apply(game, moved, 3)
	snap = mustSnapshot(t, d, guild, 0)
	if got := snap.Occupants[0]; got.ChannelID != game || !got.ChangedAt.Equal(moved) {
		t.Fatalf("move must change channel and time, got %+v", got)
	}

	apply(0, moved.Add(time.Minute), 4)
	if snap := mustSnapshot(t, d, guild, 0); len(snap.Occupants) != 0 {
		t.Fatalf("departure must remove the occupant, got %+v", snap.Occupants)
	}
}

func TestVoiceChannelAndGuildClearing(t *testing.T) {
	d := newVoiceTestDB(t)
	ctx := context.Background()
	guild, shard := rand.Int63(), testShard()
	a, b, keep, drop := rand.Int63(), rand.Int63(), rand.Int63(), rand.Int63()
	if err := d.ReplaceGuildVoiceStates(ctx, baselineFor(guild, shard, "conn",
		state.VoiceOccupant{UserID: a, ChannelID: drop, IsBot: boolPtr(false)},
		state.VoiceOccupant{UserID: b, ChannelID: keep, IsBot: boolPtr(false)},
	)); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if _, err := d.HeartbeatVoiceShard(ctx, shard, "conn", true); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if err := d.DeleteChannelVoiceStates(ctx, guild, drop); err != nil {
		t.Fatalf("delete channel: %v", err)
	}
	if snap := mustSnapshot(t, d, guild, 0); len(snap.Occupants) != 1 || snap.Occupants[0].UserID != b {
		t.Fatalf("deleted channel must lose its occupants only, got %+v", snap.Occupants)
	}

	if err := d.ClearGuildVoiceStates(ctx, guild); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if snap := mustSnapshot(t, d, guild, 0); snap.Ready || snap.NotReadyReason != voiceNotReadyNoBaseline {
		t.Fatalf("cleared guild must be unready, never empty-and-ready, got %+v", snap)
	}
}

func TestVoiceLossesKeepEarliestCutoffAndBlockReadinessUntilRebaseline(t *testing.T) {
	d := newVoiceTestDB(t)
	ctx := context.Background()
	guild, shard := rand.Int63(), testShard()
	if err := d.ReplaceGuildVoiceStates(ctx, baselineFor(guild, shard, "conn")); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if _, err := d.HeartbeatVoiceShard(ctx, shard, "conn", true); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	start := mustSnapshot(t, d, guild, 0)
	var startRevision int64
	if start.Loss != nil {
		startRevision = start.Loss.Revision
	}

	early := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)
	later := time.Now().UTC().Add(-1 * time.Minute).Truncate(time.Microsecond)
	first, err := d.RecordVoiceLoss(ctx, shard, VoiceLossPublishFailed, early)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	second, err := d.RecordVoiceLoss(ctx, shard, VoiceLossIdentifyNoResume, later)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if second.Revision <= first.Revision {
		t.Fatalf("revisions must increase: %d then %d", first.Revision, second.Revision)
	}

	snap := mustSnapshot(t, d, guild, startRevision)
	if snap.Loss == nil || snap.Loss.Revision != second.Revision || !snap.Loss.KnownThrough.Equal(early) {
		t.Fatalf("a later loss must not hide the earlier cutoff, got %+v", snap.Loss)
	}
	if !snap.Ready {
		t.Fatalf("publication and identify losses leave State's own cache usable, got %+v", snap)
	}
	if snap := mustSnapshot(t, d, guild, first.Revision); !snap.Loss.KnownThrough.Equal(later) {
		t.Fatalf("losses the caller has seen must not lower the cutoff, got %+v", snap.Loss)
	}
	if snap := mustSnapshot(t, d, guild, second.Revision); snap.Loss != nil {
		t.Fatalf("no loss after the latest revision, got %+v", snap.Loss)
	}

	future := time.Now().Add(time.Hour)
	capped, err := d.RecordVoiceLoss(ctx, shard, VoiceLossStateWriteFailed, future)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if capped.KnownThrough.After(capped.DetectedAt) {
		t.Fatalf("known-through must never pass detection, got %+v", capped)
	}
	if snap := mustSnapshot(t, d, guild, 0); snap.Ready || snap.NotReadyReason != voiceNotReadyCacheLoss {
		t.Fatalf("a failed State write must make the guild unready, got %+v", snap)
	}
	if err := d.ReplaceGuildVoiceStates(ctx, baselineFor(guild, shard, "conn")); err != nil {
		t.Fatalf("rebaseline: %v", err)
	}
	if snap := mustSnapshot(t, d, guild, 0); !snap.Ready {
		t.Fatalf("a newer baseline must restore readiness, got %+v", snap)
	}
}

func TestVoiceHeartbeatReportsRecreatedRow(t *testing.T) {
	d := newVoiceTestDB(t)
	ctx := context.Background()
	shard := testShard()

	created, err := d.HeartbeatVoiceShard(ctx, shard, "conn", true)
	if err != nil || !created {
		t.Fatalf("first heartbeat must create the row: created=%v err=%v", created, err)
	}
	created, err = d.HeartbeatVoiceShard(ctx, shard, "conn", true)
	if err != nil || created {
		t.Fatalf("second heartbeat must update: created=%v err=%v", created, err)
	}
	if _, err := d.sql.Exec(`DELETE FROM voice_shard_status WHERE shard_id = $1`, shard); err != nil {
		t.Fatalf("simulate reset: %v", err)
	}
	created, err = d.HeartbeatVoiceShard(ctx, shard, "conn", true)
	if err != nil || !created {
		t.Fatalf("a vanished row must be reported as recreated: created=%v err=%v", created, err)
	}
	at, found, err := d.GetVoiceShardHeartbeat(ctx, shard)
	if err != nil || !found || time.Since(at) > time.Minute {
		t.Fatalf("heartbeat read: at=%v found=%v err=%v", at, found, err)
	}
}
