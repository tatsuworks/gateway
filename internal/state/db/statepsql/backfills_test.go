package statepsql

import (
	"context"
	"math/rand"
	"os"
	"testing"

	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogtest/assert"
)

func newTestDB(t *testing.T) *db {
	t.Helper()
	d, err := NewDB(context.Background(), "postgresql://tatsu@localhost/state?sslmode=disable", sloghuman.Make(os.Stderr))
	if err != nil {
		t.Skipf("skipping: postgres not available: %v", err)
	}
	return d.(*db)
}

// seedMember inserts a members row, setting last_updated DB-side relative to
// the Postgres clock (now() + the given interval). It uses the DB clock, not
// Go's, so a row can be placed deterministically before or after a backfill's
// started_at (also stamped by now()) without app/DB clock skew. Rows use random
// IDs, so the INSERT never conflicts (and the BEFORE UPDATE trigger never fires
// to clobber the seeded last_updated).
func seedMember(t *testing.T, d *db, guild, user int64, interval string) {
	_, err := d.sql.Exec(
		`INSERT INTO members (guild_id, user_id, data, last_updated)
		 VALUES ($1,$2,'{}'::jsonb, now() + $3::interval)`,
		guild, user, interval,
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

// CompleteGuildBackfill must NOT delete members. The reconcile DELETE was removed:
// it could not tell a member genuinely absent from the roster apart from one whose
// upsert was silently dropped by the async batcher (logged, never retried) or that
// arrived via an out-of-order / incomplete chunk stream — so it turned a transient
// refresh gap into permanent member loss. Observed in prod: a manual RGM marked the
// backfill complete while live members vanished from `members`. Completion now only
// stamps backfilled_at; stale rows self-heal on the next member event or backfill.
func TestCompleteStampsMarkerWithoutDeleting(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	guild := rand.Int63()
	stale := rand.Int63() // last_updated before started_at (formerly a deleted "ghost")
	fresh := rand.Int63() // last_updated after started_at

	assert.Success(t, "begin", d.BeginGuildBackfill(ctx, guild))
	seedMember(t, d, guild, stale, "-1 hour")
	seedMember(t, d, guild, fresh, "1 hour")

	assert.Success(t, "complete", d.CompleteGuildBackfill(ctx, guild))

	if !memberExists(t, d, guild, stale) {
		t.Fatalf("completion must not delete a member, even one untouched since started_at")
	}
	if !memberExists(t, d, guild, fresh) {
		t.Fatalf("completion must not delete a member updated during the drain")
	}

	times, err := d.GetGuildBackfillTimes(ctx, []int64{guild})
	assert.Success(t, "get times", err)
	if _, ok := times[guild]; !ok {
		t.Fatalf("completion must stamp backfilled_at")
	}
}

func TestReconcileGuildMembers(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	guild := rand.Int63()
	keep := rand.Int63()
	drop := rand.Int63()

	seedMember(t, d, guild, keep, "0")
	seedMember(t, d, guild, drop, "0")

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
