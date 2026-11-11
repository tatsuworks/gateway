package statepsql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord/discordjson"
	"github.com/tatsuworks/gateway/internal/state"
)

// Loss reasons after which State's cached voice presence for the shard can no
// longer be trusted, so a guild is not ready again until a newer baseline.
// Other reasons (a failed publication, an identify) lose events downstream but
// leave State's own cache correct.
const (
	VoiceLossStateWriteFailed  = "state_write_failed"
	VoiceLossStateCacheReset   = "state_cache_reset"
	VoiceLossPublishFailed     = "publish_failed"
	VoiceLossIdentifyNoResume  = "identify_without_resume"
	voiceLossRetention         = "8 days"
	voiceLossLookupLimit       = 1000
	voiceNotReadyNoBaseline    = "no_baseline"
	voiceNotReadyNoHeartbeat   = "no_heartbeat"
	voiceNotReadyStale         = "stale_heartbeat"
	voiceNotReadyShardNotReady = "shard_not_ready"
	voiceNotReadyConnection    = "connection_changed"
	voiceNotReadyCacheLoss     = "cache_loss"
)

var cacheCorruptingVoiceLosses = []string{VoiceLossStateWriteFailed, VoiceLossStateCacheReset}

// SetVoiceState keeps its original signature for existing callers. It stores
// the state with database time; ApplyVoiceState carries receipt metadata.
func (db *db) SetVoiceState(ctx context.Context, guild, user int64, raw []byte) error {
	vs, err := discordjson.Encoding.DecodeVoiceState(raw)
	if err != nil {
		return xerrors.Errorf("decode voice state: %w", err)
	}
	return db.ApplyVoiceState(ctx, state.VoiceStateChange{
		GuildID:   guild,
		UserID:    user,
		ChannelID: vs.ChannelID,
		IsBot:     vs.IsBot,
		Raw:       raw,
	})
}

// voiceIsBotFallback classifies from the cached member when the event itself
// could not: Discord omits user.bot for humans, so a cached user object without
// it is a human, and no cached user object stays unknown (NULL).
const voiceIsBotFallback = `(
	SELECT CASE WHEN m.data ? 'user' THEN COALESCE((m.data->'user'->>'bot')::bool, false) END
	FROM members m
	WHERE m.guild_id = $1 AND m.user_id = $2
)`

func (db *db) ApplyVoiceState(ctx context.Context, change state.VoiceStateChange) error {
	return applyVoiceState(ctx, db.sql, change)
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func applyVoiceState(ctx context.Context, ex execer, change state.VoiceStateChange) error {
	if change.ChannelID == 0 {
		const q = `DELETE FROM voice_states WHERE guild_id = $1 AND user_id = $2`
		if _, err := ex.ExecContext(ctx, q, change.GuildID, change.UserID); err != nil {
			return xerrors.Errorf("exec delete: %w", err)
		}
		return nil
	}

	// changed_at only moves when the location does: mute, deafen and other
	// same-channel updates keep the time the member arrived.
	q := `
INSERT INTO
	voice_states (guild_id, user_id, data, channel_id, is_bot, changed_at, discord_seq)
VALUES
	($1, $2, $3, $4, COALESCE($5, ` + voiceIsBotFallback + `), COALESCE($6, now()), $7)
ON CONFLICT (guild_id, user_id)
DO UPDATE SET
	data = excluded.data,
	channel_id = excluded.channel_id,
	is_bot = COALESCE(excluded.is_bot, voice_states.is_bot),
	changed_at = CASE
		WHEN voice_states.channel_id = excluded.channel_id THEN voice_states.changed_at
		ELSE excluded.changed_at
	END,
	discord_seq = excluded.discord_seq
`
	_, err := ex.ExecContext(ctx, q,
		change.GuildID, change.UserID, voiceRaw(change.Raw), change.ChannelID,
		nullBool(change.IsBot), nullTime(change.ChangedAt), change.DiscordSeq,
	)
	if err != nil {
		return xerrors.Errorf("exec upsert: %w", err)
	}
	return nil
}

func (db *db) ReplaceGuildVoiceStates(ctx context.Context, baseline state.VoiceBaseline) (retErr error) {
	tx, err := db.sql.BeginTxx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("begin: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM voice_states WHERE guild_id = $1`, baseline.GuildID); err != nil {
		return xerrors.Errorf("clear voice states: %w", err)
	}
	for _, occupant := range baseline.Occupants {
		if err := applyVoiceState(ctx, tx, state.VoiceStateChange{
			GuildID:    baseline.GuildID,
			UserID:     occupant.UserID,
			ChannelID:  occupant.ChannelID,
			IsBot:      occupant.IsBot,
			ChangedAt:  baseline.ReceivedAt,
			DiscordSeq: baseline.DiscordSeq,
			Raw:        baseline.Raws[occupant.UserID],
		}); err != nil {
			return xerrors.Errorf("insert voice state: %w", err)
		}
	}

	const readiness = `
INSERT INTO
	voice_guild_readiness (guild_id, shard_id, connection_id, baseline_at)
VALUES
	($1, $2, $3, now())
ON CONFLICT (guild_id)
DO UPDATE SET
	shard_id = excluded.shard_id,
	connection_id = excluded.connection_id,
	baseline_at = excluded.baseline_at
`
	if _, err := tx.ExecContext(ctx, readiness, baseline.GuildID, baseline.ShardID, baseline.ConnectionID); err != nil {
		return xerrors.Errorf("mark guild ready: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("commit: %w", err)
	}
	return nil
}

func (db *db) ClearGuildVoiceStates(ctx context.Context, guild int64) error {
	// One statement, so readiness can never outlive the presence it vouched for.
	const q = `
WITH cleared AS (
	DELETE FROM voice_states WHERE guild_id = $1
)
DELETE FROM voice_guild_readiness WHERE guild_id = $1
`
	if _, err := db.sql.ExecContext(ctx, q, guild); err != nil {
		return xerrors.Errorf("exec delete: %w", err)
	}
	return nil
}

func (db *db) DeleteChannelVoiceStates(ctx context.Context, guild, channel int64) error {
	const q = `DELETE FROM voice_states WHERE guild_id = $1 AND channel_id = $2`
	if _, err := db.sql.ExecContext(ctx, q, guild, channel); err != nil {
		return xerrors.Errorf("exec delete: %w", err)
	}
	return nil
}

func (db *db) HeartbeatVoiceShard(ctx context.Context, shard int, connectionID string, ready bool) (bool, error) {
	// xmax = 0 only for a freshly inserted row. A connection that has already
	// heartbeated and then sees an insert has lost its row to a cache reset.
	const q = `
INSERT INTO
	voice_shard_status (shard_id, connection_id, ready, heartbeat_at)
VALUES
	($1, $2, $3, now())
ON CONFLICT (shard_id)
DO UPDATE SET
	connection_id = excluded.connection_id,
	ready = excluded.ready,
	heartbeat_at = excluded.heartbeat_at
RETURNING (xmax = 0) AS created
`
	var created bool
	if err := db.sql.GetContext(ctx, &created, q, shard, connectionID, ready); err != nil {
		return false, xerrors.Errorf("exec upsert: %w", err)
	}
	return created, nil
}

func (db *db) GetVoiceShardHeartbeat(ctx context.Context, shard int) (time.Time, bool, error) {
	const q = `SELECT heartbeat_at FROM voice_shard_status WHERE shard_id = $1`
	var at time.Time
	err := db.sql.GetContext(ctx, &at, q, shard)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, xerrors.Errorf("exec select: %w", err)
	}
	return at, true, nil
}

func (db *db) RecordVoiceLoss(ctx context.Context, shard int, reason string, knownThrough time.Time) (*state.VoiceLoss, error) {
	// known_through can never be later than the moment the loss is recorded.
	const q = `
WITH pruned AS (
	DELETE FROM voice_shard_losses WHERE detected_at < now() - $4::interval
)
INSERT INTO
	voice_shard_losses (shard_id, reason, known_through)
VALUES
	($1, $2, LEAST($3::timestamptz, now()))
RETURNING revision, reason, known_through, detected_at
`
	var row voiceLossRow
	if err := db.sql.GetContext(ctx, &row, q, shard, reason, knownThrough.UTC(), voiceLossRetention); err != nil {
		return nil, xerrors.Errorf("exec insert: %w", err)
	}
	return &state.VoiceLoss{
		Revision:     row.Revision,
		ShardID:      shard,
		Reason:       row.Reason,
		KnownThrough: row.KnownThrough,
		DetectedAt:   row.DetectedAt,
	}, nil
}

type voiceReadinessRow struct {
	ShardID      int       `db:"shard_id"`
	ConnectionID string    `db:"connection_id"`
	BaselineAt   time.Time `db:"baseline_at"`
}

type voiceShardRow struct {
	ConnectionID string    `db:"connection_id"`
	Ready        bool      `db:"ready"`
	HeartbeatAt  time.Time `db:"heartbeat_at"`
}

type voiceOccupantRow struct {
	UserID     int64        `db:"user_id"`
	ChannelID  int64        `db:"channel_id"`
	IsBot      sql.NullBool `db:"is_bot"`
	ChangedAt  time.Time    `db:"changed_at"`
	DiscordSeq int64        `db:"discord_seq"`
}

type voiceLossRow struct {
	Revision     int64     `db:"revision"`
	Reason       string    `db:"reason"`
	KnownThrough time.Time `db:"known_through"`
	DetectedAt   time.Time `db:"detected_at"`
}

func (db *db) GetGuildVoiceSnapshot(ctx context.Context, guild int64, afterLossRevision int64, maxHeartbeatAge time.Duration) (_ *state.VoiceSnapshot, retErr error) {
	// One repeatable-read snapshot, so presence and the evidence vouching for
	// it are read at the same instant.
	tx, err := db.sql.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, xerrors.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	snap := &state.VoiceSnapshot{GuildID: guild, ShardID: -1}
	if err := tx.GetContext(ctx, &snap.ObservedAt, `SELECT now()`); err != nil {
		return nil, xerrors.Errorf("read clock: %w", err)
	}

	var readiness voiceReadinessRow
	err = tx.GetContext(ctx, &readiness, `SELECT shard_id, connection_id, baseline_at FROM voice_guild_readiness WHERE guild_id = $1`, guild)
	if errors.Is(err, sql.ErrNoRows) {
		snap.NotReadyReason = voiceNotReadyNoBaseline
		return snap, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("read readiness: %w", err)
	}
	snap.ShardID = readiness.ShardID
	snap.ConnectionID = readiness.ConnectionID

	var occupants []voiceOccupantRow
	if err := tx.SelectContext(ctx, &occupants, `
SELECT user_id, channel_id, is_bot, changed_at, discord_seq
FROM voice_states
WHERE guild_id = $1 AND channel_id <> 0
ORDER BY user_id
`, guild); err != nil {
		return nil, xerrors.Errorf("read occupants: %w", err)
	}
	snap.Occupants = make([]state.VoiceOccupant, 0, len(occupants))
	classified := true
	for _, o := range occupants {
		occupant := state.VoiceOccupant{UserID: o.UserID, ChannelID: o.ChannelID, ChangedAt: o.ChangedAt, DiscordSeq: o.DiscordSeq}
		if o.IsBot.Valid {
			isBot := o.IsBot.Bool
			occupant.IsBot = &isBot
		} else {
			classified = false
		}
		snap.Occupants = append(snap.Occupants, occupant)
	}

	var losses []voiceLossRow
	if err := tx.SelectContext(ctx, &losses, `
SELECT revision, reason, known_through, detected_at
FROM voice_shard_losses
WHERE shard_id = $1 AND revision > $2
ORDER BY revision DESC
LIMIT $3
`, readiness.ShardID, afterLossRevision, voiceLossLookupLimit); err != nil {
		return nil, xerrors.Errorf("read losses: %w", err)
	}
	if len(losses) > 0 {
		latest := losses[0]
		loss := &state.VoiceLoss{
			Revision:     latest.Revision,
			ShardID:      readiness.ShardID,
			Reason:       latest.Reason,
			KnownThrough: latest.KnownThrough,
			DetectedAt:   latest.DetectedAt,
		}
		for _, l := range losses[1:] {
			if l.KnownThrough.Before(loss.KnownThrough) {
				loss.KnownThrough = l.KnownThrough
			}
		}
		snap.Loss = loss
	}

	var lastCacheLoss sql.NullTime
	if err := tx.GetContext(ctx, &lastCacheLoss, `
SELECT max(detected_at) FROM voice_shard_losses WHERE shard_id = $1 AND reason = ANY($2)
`, readiness.ShardID, pq.Array(cacheCorruptingVoiceLosses)); err != nil {
		return nil, xerrors.Errorf("read cache losses: %w", err)
	}

	var shard voiceShardRow
	err = tx.GetContext(ctx, &shard, `SELECT connection_id, ready, heartbeat_at FROM voice_shard_status WHERE shard_id = $1`, readiness.ShardID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		snap.NotReadyReason = voiceNotReadyNoHeartbeat
	case err != nil:
		return nil, xerrors.Errorf("read shard status: %w", err)
	case snap.ObservedAt.Sub(shard.HeartbeatAt) > maxHeartbeatAge:
		snap.NotReadyReason = voiceNotReadyStale
	case !shard.Ready:
		snap.NotReadyReason = voiceNotReadyShardNotReady
	case shard.ConnectionID != readiness.ConnectionID:
		snap.NotReadyReason = voiceNotReadyConnection
	case lastCacheLoss.Valid && !lastCacheLoss.Time.Before(readiness.BaselineAt):
		snap.NotReadyReason = voiceNotReadyCacheLoss
	default:
		snap.Ready = true
		snap.Complete = classified
	}

	return snap, nil
}

func voiceRaw(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return []byte(bytesToString(raw))
}

func nullBool(b *bool) sql.NullBool {
	if b == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *b, Valid: true}
}

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}
