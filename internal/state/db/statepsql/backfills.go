package statepsql

import (
	"context"
	"time"

	"github.com/lib/pq"
	"golang.org/x/xerrors"
)

// BeginGuildBackfill stamps started_at for a fresh backfill, preserving any
// prior backfilled_at (an in-flight re-backfill does not invalidate the
// previous completed one).
func (db *db) BeginGuildBackfill(ctx context.Context, guild int64) error {
	const q = `
INSERT INTO guild_backfills (guild_id, started_at)
VALUES ($1, now())
ON CONFLICT (guild_id)
DO UPDATE SET started_at = now()
`
	if _, err := db.sql.ExecContext(ctx, q, guild); err != nil {
		return xerrors.Errorf("begin guild backfill: %w", err)
	}
	return nil
}

// CompleteGuildBackfill stamps backfilled_at to mark the guild's roster fetched.
//
// It deliberately does NOT prune members absent from the backfill. The previous
// reconcile DELETE (members untouched since started_at) could not distinguish a
// genuinely-departed member from one whose upsert was dropped by the async member
// batcher (failed batches are logged, never retried) or that arrived via an
// out-of-order / interrupted chunk stream — so a transient refresh gap became
// permanent member loss (observed in prod: a manual RGM marked the backfill
// complete while live members vanished from `members`). Stale rows are left to
// self-heal instead: live GUILD_MEMBER_REMOVE events prune departures while
// connected, and the UNLOGGED members table is reset wholesale on an unclean PG
// restart. The bounded drift this leaves is the same drift the no-expiry backfill
// skip already accepts.
func (db *db) CompleteGuildBackfill(ctx context.Context, guild int64) error {
	const upd = `UPDATE guild_backfills SET backfilled_at = now() WHERE guild_id = $1`
	if _, err := db.sql.ExecContext(ctx, upd, guild); err != nil {
		return xerrors.Errorf("stamp backfilled_at: %w", err)
	}
	return nil
}

// ReconcileGuildMembers handles small guilds whose full roster arrives in the
// GUILD_CREATE payload (no chunks). It deletes members not in the roster and
// stamps a completed marker, in one transaction. The delete is exact (roster
// IDs, no timestamps) so it is immune to member-batcher ordering.
func (db *db) ReconcileGuildMembers(ctx context.Context, guild int64, roster []int64) error {
	// Guard against an empty roster: `user_id != ALL($2)` with an empty array
	// matches every row, which would delete the entire guild's members.
	if len(roster) == 0 {
		return nil
	}

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	const del = `DELETE FROM members WHERE guild_id = $1 AND user_id != ALL($2)`
	if _, err := tx.ExecContext(ctx, del, guild, pq.Array(roster)); err != nil {
		return xerrors.Errorf("reconcile delete: %w", err)
	}

	const mark = `
INSERT INTO guild_backfills (guild_id, started_at, backfilled_at)
VALUES ($1, now(), now())
ON CONFLICT (guild_id)
DO UPDATE SET started_at = now(), backfilled_at = now()
`
	if _, err := tx.ExecContext(ctx, mark, guild); err != nil {
		return xerrors.Errorf("stamp marker: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("commit: %w", err)
	}
	return nil
}

// GetGuildBackfillTimes bulk-reads completion times for the given guilds,
// excluding never-completed (NULL backfilled_at) markers. The session applies
// the per-guild jittered staleness threshold to the returned times.
func (db *db) GetGuildBackfillTimes(ctx context.Context, guilds []int64) (map[int64]time.Time, error) {
	const q = `
SELECT guild_id, backfilled_at
FROM guild_backfills
WHERE guild_id = ANY($1) AND backfilled_at IS NOT NULL
`
	rows, err := db.sql.QueryContext(ctx, q, pq.Array(guilds))
	if err != nil {
		return nil, xerrors.Errorf("query backfill times: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]time.Time)
	for rows.Next() {
		var (
			id int64
			t  time.Time
		)
		if err := rows.Scan(&id, &t); err != nil {
			return nil, xerrors.Errorf("scan backfill time: %w", err)
		}
		out[id] = t
	}
	if err := rows.Err(); err != nil {
		return nil, xerrors.Errorf("iterate backfill times: %w", err)
	}
	return out, nil
}
