package statefdb

import (
	"context"
	"time"
)

// The backfill-marker optimization is PSQL-only (see repo CLAUDE.md). FDB
// deployments always take the RGM slow path and get no reconciliation delete.

func (db *DB) BeginGuildBackfill(ctx context.Context, guild int64) error { return nil }

func (db *DB) CompleteGuildBackfill(ctx context.Context, guild int64) error { return nil }

func (db *DB) ReconcileGuildMembers(ctx context.Context, guild int64, roster []int64) error {
	return nil
}

func (db *DB) GetGuildBackfillTimes(ctx context.Context, guilds []int64) (map[int64]time.Time, error) {
	return nil, nil
}
