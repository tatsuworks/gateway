package statepsql

import (
	"context"
	"math/rand"
	"os"
	"testing"
	"time"

	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogtest/assert"
)

func newTestDB(t *testing.T) *db {
	d, err := NewDB(context.Background(), "postgresql://tatsu@localhost/state?sslmode=disable", sloghuman.Make(os.Stderr))
	assert.Success(t, "open postgres", err)
	return d.(*db)
}

// seedMember inserts a members row with an explicit last_updated.
func seedMember(t *testing.T, d *db, guild, user int64, lastUpdated time.Time) {
	_, err := d.sql.Exec(
		`INSERT INTO members (guild_id, user_id, data, last_updated) VALUES ($1,$2,'{}'::jsonb,$3)
		 ON CONFLICT (guild_id, user_id) DO UPDATE SET last_updated = EXCLUDED.last_updated`,
		guild, user, lastUpdated,
	)
	assert.Success(t, "seed member", err)
}

func memberExists(t *testing.T, d *db, guild, user int64) bool {
	var exists bool
	err := d.sql.Get(&exists, `SELECT EXISTS(SELECT 1 FROM members WHERE guild_id=$1 AND user_id=$2)`, guild, user)
	assert.Success(t, "exists member", err)
	return exists
}

func TestBeginPreservesPriorBackfilledAt(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	guild := rand.Int63()

	assert.Success(t, "begin", d.BeginGuildBackfill(ctx, guild))
	assert.Success(t, "complete", d.CompleteGuildBackfill(ctx, guild))

	// A new begin must not clear the prior backfilled_at.
	assert.Success(t, "begin again", d.BeginGuildBackfill(ctx, guild))

	times, err := d.GetGuildBackfillTimes(ctx, []int64{guild})
	assert.Success(t, "get times", err)
	if _, ok := times[guild]; !ok {
		t.Fatalf("expected backfilled_at to survive a second begin")
	}
}

func TestGetGuildBackfillTimesExcludesIncomplete(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	done := rand.Int63()
	pending := rand.Int63()

	assert.Success(t, "begin done", d.BeginGuildBackfill(ctx, done))
	assert.Success(t, "complete done", d.CompleteGuildBackfill(ctx, done))
	assert.Success(t, "begin pending", d.BeginGuildBackfill(ctx, pending))

	times, err := d.GetGuildBackfillTimes(ctx, []int64{done, pending})
	assert.Success(t, "get times", err)
	if _, ok := times[done]; !ok {
		t.Fatalf("completed guild missing from times")
	}
	if _, ok := times[pending]; ok {
		t.Fatalf("incomplete guild (NULL backfilled_at) must be excluded")
	}
}

func TestCompleteReconciliationDelete(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	guild := rand.Int63()
	ghost := rand.Int63()
	fresh := rand.Int63()

	// ghost predates this backfill; fresh is updated during the drain.
	assert.Success(t, "begin", d.BeginGuildBackfill(ctx, guild))
	seedMember(t, d, guild, ghost, time.Now().Add(-time.Hour)) // before started_at
	seedMember(t, d, guild, fresh, time.Now().Add(time.Hour))  // after started_at

	assert.Success(t, "complete", d.CompleteGuildBackfill(ctx, guild))

	if memberExists(t, d, guild, ghost) {
		t.Fatalf("ghost member (untouched since started_at) should be deleted")
	}
	if !memberExists(t, d, guild, fresh) {
		t.Fatalf("member updated during drain should survive")
	}
}

func TestReconcileGuildMembers(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	guild := rand.Int63()
	keep := rand.Int63()
	drop := rand.Int63()

	seedMember(t, d, guild, keep, time.Now())
	seedMember(t, d, guild, drop, time.Now())

	assert.Success(t, "reconcile", d.ReconcileGuildMembers(ctx, guild, []int64{keep}))

	if !memberExists(t, d, guild, keep) {
		t.Fatalf("member in roster should survive")
	}
	if memberExists(t, d, guild, drop) {
		t.Fatalf("member absent from roster should be deleted")
	}

	times, err := d.GetGuildBackfillTimes(ctx, []int64{guild})
	assert.Success(t, "get times", err)
	if _, ok := times[guild]; !ok {
		t.Fatalf("reconcile should stamp a completed marker")
	}
}
