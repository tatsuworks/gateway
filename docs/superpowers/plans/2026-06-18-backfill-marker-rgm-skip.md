# Backfill-Completion Marker for RGM Skip — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace always-on `Request Guild Members` (RGM) on GUILD_CREATE with a completion-marker mechanism: stamp a `guild_backfills` marker when a backfill finishes, read markers in bulk once per connect, and skip/recur RGM on a jittered staleness bound — while reconciling departed ("ghost") members.

**Architecture:** A new `UNLOGGED` `guild_backfills(guild_id, started_at, backfilled_at)` table records backfill lifecycle. `GUILD_MEMBERS_CHUNK` decoding gains `chunk_index`/`chunk_count`; the handler stamps `started_at` on the first chunk and `backfilled_at` (plus a reconciliation delete) on the last. On READY the session bulk-loads fresh markers into an in-memory `backfilled` set; the GUILD_CREATE path then either skips RGM (fresh marker), reconciles a small guild straight from the payload roster, or fires RGM (default). Decision is in-memory; DB work happens at most once per guild per staleness window.

**Tech Stack:** Go 1.25, PostgreSQL (`lib/pq` + `jmoiron/sqlx`), custom ETF decoder (`discord/discordetf`), `jsoniter` JSON decoder (`discord/discordjson`), `cdr.dev/slog`, `golang.org/x/xerrors`.

## Global Constraints

- **Baseline is `master`, not `feat/gate-rgm-on-reidentify`.** This branch never had the `GuildHasMembers` EXISTS probe or the 5-value `GuildCreate`. The design's "remove `GuildHasMembers`" / "drop the 5-value return" instructions are **no-ops here** — we implement the end state directly. There is nothing to delete.
- **PSQL is the source of truth.** All marker logic targets `statepsql`. The `statefdb` backend implements the new interface methods as no-ops (writers return `nil`, `GetGuildBackfillTimes` returns `nil`), so FDB deployments always take the RGM slow path. (Per repo CLAUDE.md: cache-dependent optimizations target PSQL only.)
- **`guild_backfills` is `UNLOGGED`**, matching `members`: a PSQL crash that wipes cache tables also wipes markers, so guilds re-read as cold. Markers must never outlive the member data they vouch for.
- **Timestamps come from the Postgres clock** (`now()`), the same clock as the `update_members_last_updated` trigger the reconciliation delete compares against. Never compare against an app-side clock.
- **Fail open.** Any marker read/write error logs and falls back to RGM (empty set / slow path). Never block event processing on marker errors.
- **`LargeThreshold = 250`** (Discord's `large_threshold` in the IDENTIFY payload). Small-guild completeness predicate: `0 < MemberCount <= LargeThreshold && ReceivedMembers >= MemberCount`.
- **`BACKFILL_STALENESS_HOURS`** env knob, default **24**. Effective per-guild threshold is `staleness × jitter(guild_id)`, `jitter ∈ [0.75, 1.25]`, derived from a deterministic hash of the guild ID.
- **REVISION 2026-06-18 — no-expiry skip.** Task 5's skip decision is now no-expiry: skip RGM for any guild with `backfilled_at IS NOT NULL`, regardless of age (reconnect-storm speed is the priority — see the design doc's 2026-06-18 revision banner). The READY preload no longer applies the `isFresh` staleness filter; `backfillStalenessWindow`/`isFresh`/`jitterFactor` are retained only for the forthcoming Phase 3b background sweep's cadence, not the skip. Step 6 below reflects the original expiring model; the shipped code drops the filter.
- **Statepsql tests are integration tests** requiring a live Postgres at `postgresql://tatsu@localhost/state?sslmode=disable` (see `channels_test.go`). They are skipped by reviewers without a DB but MUST be run by the implementer against a real DB before marking the DB task done.

---

## File Structure

**Create:**
- `internal/state/db/statepsql/backfills.go` — psql impl of the four marker methods.
- `internal/state/db/statepsql/backfills_test.go` — integration tests for the marker methods.
- `internal/state/db/statefdb/backfills.go` — FDB no-op impl.
- `discord/discordetf/member_test.go` — ETF chunk decode test (new file; no ETF tests exist today).
- `discord/discordjson/member_test.go` — JSON chunk decode test.
- `internal/gatewayws/backfill.go` — `LargeThreshold` const, `membersComplete`, `jitterFactor`, `isFresh`, `backfillStalenessWindow` helpers.
- `internal/gatewayws/backfill_test.go` — unit tests for the pure helpers.

**Modify:**
- `discord/types.go:49-52` — add `ChunkIndex, ChunkCount int` to `MemberChunk`.
- `discord/discordetf/member.go:9-47` — extract `chunk_index`/`chunk_count`; make the `default` case skip unknown values.
- `discord/discordjson/member.go:10-29` — extract `chunk_index`/`chunk_count`.
- `internal/state/state.go:44-54` — add four interface methods + `time` import.
- `internal/state/db/statepsql/init.sql` — add `guild_backfills` table.
- `internal/state/db/statepsql/down.sql` — add drop.
- `handler/helpers.go:11-13,23-28` — expand `EventPayload`; route GUILD_CREATE/UPDATE to the new `GuildCreate` signature.
- `handler/guild.go:11-102` — `GuildCreate` returns `(*EventPayload, error)`.
- `handler/member.go:10-22` — `MemberChunk` stamps begin/complete markers.
- `internal/gatewayws/ws.go:38-90` — add `backfilled` field to `Session`.
- `internal/gatewayws/ws.go:320-325` — replace unconditional RGM with the skip switch.
- `internal/gatewayws/ws.go:403-419` — preload markers in the READY branch.
- `internal/gatewayws/write.go:113` — use `LargeThreshold` const for IDENTIFY's `large_threshold`.

---

## Task 1: Decode `chunk_index` / `chunk_count`

**Files:**
- Modify: `discord/types.go:49-52`
- Modify: `discord/discordetf/member.go:9-47`
- Modify: `discord/discordjson/member.go:10-29`
- Test: `discord/discordetf/member_test.go` (create)
- Test: `discord/discordjson/member_test.go` (create)

**Interfaces:**
- Produces: `discord.MemberChunk` now carries `ChunkIndex int` and `ChunkCount int`. Task 4 reads them in `handler.MemberChunk`.

- [ ] **Step 1: Add the fields to the struct**

In `discord/types.go`, change the `MemberChunk` struct (currently lines 49-52):

```go
type MemberChunk struct {
	GuildID    int64
	Members    map[int64][]byte
	ChunkIndex int
	ChunkCount int
}
```

- [ ] **Step 2: Write the failing ETF decode test**

Create `discord/discordetf/member_test.go`. The helper builds a real ETF map by appending bytes using the package-private tag constants (test is in package `discordetf`), so it stays correct relative to the decoder. The map has keys in an order where `chunk_index`/`chunk_count` and an unknown `nonce` key follow `members` — this proves the decoder both extracts the new fields and skips unknown values without desyncing.

```go
package discordetf

import (
	"encoding/binary"
	"testing"

	"cdr.dev/slog/sloggers/slogtest/assert"
)

// atom encodes an ETF atom term: tag + 2-byte big-endian length + bytes.
func atom(s string) []byte {
	b := []byte{ettAtom, 0, 0}
	binary.BigEndian.PutUint16(b[1:3], uint16(len(s)))
	return append(b, []byte(s)...)
}

// smallInt encodes an ETF small-integer term: tag + 1 byte.
func smallInt(n byte) []byte { return []byte{ettSmallInteger, n} }

// smallBig encodes a non-negative ETF small-big term: tag + len + sign(0) + 1 magnitude byte.
func smallBig(n byte) []byte { return []byte{ettSmallBig, 0x01, 0x00, n} }

// binaryTerm encodes an ETF binary term: tag + 4-byte big-endian length + bytes.
func binaryTerm(s string) []byte {
	b := []byte{ettBinary, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(b[1:5], uint32(len(s)))
	return append(b, []byte(s)...)
}

func TestDecodeMemberChunkFields(t *testing.T) {
	var buf []byte
	// map header: ettMap + 4-byte big-endian arity (5 pairs)
	hdr := []byte{ettMap, 0, 0, 0, 5}
	buf = append(buf, hdr...)
	buf = append(buf, atom("guild_id")...)
	buf = append(buf, smallBig(7)...)
	buf = append(buf, atom("members")...)
	buf = append(buf, ettNil) // empty members list
	buf = append(buf, atom("nonce")...)
	buf = append(buf, binaryTerm("abc")...) // unknown key, must be skipped
	buf = append(buf, atom("chunk_index")...)
	buf = append(buf, smallInt(1)...)
	buf = append(buf, atom("chunk_count")...)
	buf = append(buf, smallInt(3)...)

	mc, err := (decoder{}).DecodeMemberChunk(buf)
	assert.Success(t, "decode member chunk", err)
	assert.Equal(t, "guild id", int64(7), mc.GuildID)
	assert.Equal(t, "chunk index", 1, mc.ChunkIndex)
	assert.Equal(t, "chunk count", 3, mc.ChunkCount)
}
```

- [ ] **Step 3: Run it to confirm it fails**

Run: `go test ./discord/discordetf/ -run TestDecodeMemberChunkFields -v`
Expected: FAIL — `chunk_index`/`chunk_count` are not extracted (`ChunkIndex`/`ChunkCount` are 0), and with the current empty `default:` the decoder desyncs after the unknown `nonce` key and returns a decode error.

- [ ] **Step 4: Implement ETF extraction + skip-unknown**

In `discord/discordetf/member.go`, replace the `switch key` block inside `DecodeMemberChunk` (currently lines 28-42) with:

```go
		switch key {
		case "guild_id":
			mc.GuildID, err = d.readSmallBigWithTagToInt64()
			if err != nil {
				return nil, xerrors.Errorf("extract guild_id from map: %w", err)
			}

		case "members":
			mc.Members, err = d.readListIntoMapByID()
			if err != nil {
				return nil, xerrors.Errorf("extract members list from map: %w", err)
			}

		case "chunk_index":
			mc.ChunkIndex, err = d.readIntWithTagIntoInt()
			if err != nil {
				return nil, xerrors.Errorf("extract chunk_index from map: %w", err)
			}

		case "chunk_count":
			mc.ChunkCount, err = d.readIntWithTagIntoInt()
			if err != nil {
				return nil, xerrors.Errorf("extract chunk_count from map: %w", err)
			}

		default:
			if err := d.readTerm(); err != nil {
				return nil, xerrors.Errorf("skip member chunk key %q: %w", key, err)
			}
		}
```

(The empty `default:` was a latent desync bug — it left unknown values unconsumed. `readTerm` is the canonical value-skipper used elsewhere, e.g. `readUntilData`. This fix is required for correct extraction regardless of Discord's key order.)

- [ ] **Step 5: Run the ETF test to confirm it passes**

Run: `go test ./discord/discordetf/ -run TestDecodeMemberChunkFields -v`
Expected: PASS

- [ ] **Step 6: Write the failing JSON decode test**

Create `discord/discordjson/member_test.go`:

```go
package discordjson

import (
	"testing"

	"cdr.dev/slog/sloggers/slogtest/assert"
)

func TestDecodeMemberChunkFields(t *testing.T) {
	buf := []byte(`{"guild_id":"7","members":[],"chunk_index":1,"chunk_count":3}`)

	mc, err := (decoder{}).DecodeMemberChunk(buf)
	assert.Success(t, "decode member chunk", err)
	assert.Equal(t, "guild id", int64(7), mc.GuildID)
	assert.Equal(t, "chunk index", 1, mc.ChunkIndex)
	assert.Equal(t, "chunk count", 3, mc.ChunkCount)
}
```

- [ ] **Step 7: Run it to confirm it fails**

Run: `go test ./discord/discordjson/ -run TestDecodeMemberChunkFields -v`
Expected: FAIL — `ChunkIndex`/`ChunkCount` are 0.

- [ ] **Step 8: Implement JSON extraction**

In `discord/discordjson/member.go`, inside `DecodeMemberChunk`, after the `mc.GuildID, err = snowflakeFromObject(...)` block (currently lines 23-26) and before `return &mc, nil`, add:

```go
	mc.ChunkIndex = jsoniter.Get(buf, "chunk_index").ToInt()
	mc.ChunkCount = jsoniter.Get(buf, "chunk_count").ToInt()
```

(`jsoniter.Get` returns 0 for an absent field, so a payload missing the field yields `ChunkCount == 0` — which the Task 4 guard treats as "no completion marker".)

- [ ] **Step 9: Run the JSON test to confirm it passes**

Run: `go test ./discord/discordjson/ -run TestDecodeMemberChunkFields -v`
Expected: PASS

- [ ] **Step 10: Build the decoder packages**

Run: `go build ./discord/...`
Expected: no output (success).

- [ ] **Step 11: Commit**

```bash
git add discord/types.go discord/discordetf/member.go discord/discordetf/member_test.go discord/discordjson/member.go discord/discordjson/member_test.go
git commit -m "feat(discord): decode chunk_index/chunk_count on member chunks"
```

---

## Task 2: `guild_backfills` schema

**Files:**
- Modify: `internal/state/db/statepsql/init.sql`
- Modify: `internal/state/db/statepsql/down.sql`

**Interfaces:**
- Produces: a `guild_backfills(guild_id PK, started_at, backfilled_at)` table that Task 3's psql methods read and write.

- [ ] **Step 1: Add the table to init.sql**

Append to `internal/state/db/statepsql/init.sql`:

```sql
CREATE UNLOGGED TABLE
IF NOT EXISTS guild_backfills
(
	"guild_id" int8 NOT NULL,
	"started_at" timestamp NOT NULL DEFAULT now(),
	"backfilled_at" timestamp NULL,
	PRIMARY KEY
("guild_id")
);
```

(No secondary index: lookups are by PK or `guild_id = ANY(...)`, both served by the PK. The design deliberately omits a `(guild_id, last_updated)` index on `members` — do not add one.)

- [ ] **Step 2: Add the drop to down.sql**

In `internal/state/db/statepsql/down.sql`, add a line (order doesn't matter — no FKs):

```sql
drop TABLE guild_backfills;
```

- [ ] **Step 3: Apply the schema to your local DB**

Run (against the same DB the tests use):
```bash
psql "postgresql://tatsu@localhost/state?sslmode=disable" -f internal/state/db/statepsql/init.sql
```
Expected: `CREATE TABLE` (or no error if it already exists). Verify:
```bash
psql "postgresql://tatsu@localhost/state?sslmode=disable" -c '\d guild_backfills'
```
Expected: shows the three columns with `guild_id` as PK.

- [ ] **Step 4: Commit**

```bash
git add internal/state/db/statepsql/init.sql internal/state/db/statepsql/down.sql
git commit -m "feat(statepsql): add guild_backfills marker table"
```

---

## Task 3: DB interface + statepsql impl + statefdb no-op

This is one task because adding methods to the `state.DB` interface breaks the build until **both** backends implement them. All three files change together to keep the build green.

**Files:**
- Modify: `internal/state/state.go:44-54` (+ `time` import)
- Create: `internal/state/db/statepsql/backfills.go`
- Create: `internal/state/db/statefdb/backfills.go`
- Test: `internal/state/db/statepsql/backfills_test.go` (create)

**Interfaces:**
- Consumes: the `guild_backfills` table (Task 2); the `members` table with its `last_updated` column + `update_members_last_updated` trigger (existing).
- Produces (the four `state.DB` methods, used by Tasks 4 and 5):
  - `BeginGuildBackfill(ctx context.Context, guild int64) error`
  - `CompleteGuildBackfill(ctx context.Context, guild int64) error`
  - `ReconcileGuildMembers(ctx context.Context, guild int64, roster []int64) error`
  - `GetGuildBackfillTimes(ctx context.Context, guilds []int64) (map[int64]time.Time, error)`

- [ ] **Step 1: Add the methods to the interface**

In `internal/state/state.go`, ensure `"time"` is in the import block, then add these four lines right after the `SearchGuildMembers(...)` line (currently line 54):

```go
	BeginGuildBackfill(ctx context.Context, guild int64) error
	CompleteGuildBackfill(ctx context.Context, guild int64) error
	ReconcileGuildMembers(ctx context.Context, guild int64, roster []int64) error
	GetGuildBackfillTimes(ctx context.Context, guilds []int64) (map[int64]time.Time, error)
```

- [ ] **Step 2: Confirm the build now fails (interface unsatisfied)**

Run: `go build ./internal/state/...`
Expected: FAIL — `*db` and `*DB` no longer satisfy `state.DB` (missing methods). This confirms `var _ state.DB = &db{}` in `db.go` is doing its job.

- [ ] **Step 3: Write the statepsql implementation**

Create `internal/state/db/statepsql/backfills.go`:

```go
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

// CompleteGuildBackfill removes members untouched since this backfill's
// started_at (departed ghosts) and stamps backfilled_at, in one transaction.
func (db *db) CompleteGuildBackfill(ctx context.Context, guild int64) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	const del = `
DELETE FROM members
WHERE guild_id = $1
  AND last_updated < (SELECT started_at FROM guild_backfills WHERE guild_id = $1)
`
	if _, err := tx.ExecContext(ctx, del, guild); err != nil {
		return xerrors.Errorf("reconcile delete: %w", err)
	}

	const upd = `UPDATE guild_backfills SET backfilled_at = now() WHERE guild_id = $1`
	if _, err := tx.ExecContext(ctx, upd, guild); err != nil {
		return xerrors.Errorf("stamp backfilled_at: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("commit: %w", err)
	}
	return nil
}

// ReconcileGuildMembers handles small guilds whose full roster arrives in the
// GUILD_CREATE payload (no chunks). It deletes members not in the roster and
// stamps a completed marker, in one transaction. The delete is exact (roster
// IDs, no timestamps) so it is immune to member-batcher ordering.
func (db *db) ReconcileGuildMembers(ctx context.Context, guild int64, roster []int64) error {
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
```

- [ ] **Step 4: Write the statefdb no-op implementation**

Create `internal/state/db/statefdb/backfills.go`:

```go
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
```

- [ ] **Step 5: Confirm the build is green again**

Run: `go build ./...`
Expected: no output (success). Both backends now satisfy `state.DB`.

- [ ] **Step 6: Write the statepsql integration tests**

Create `internal/state/db/statepsql/backfills_test.go`. Follows the `channels_test.go` pattern (live DB, `rand` IDs, `assert`). Uses raw `db.sql` to seed `members` rows with controlled `last_updated` and to read back marker state, exercising the real `*db`.

```go
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
```

- [ ] **Step 7: Run the statepsql tests against a live DB**

Run: `go test ./internal/state/db/statepsql/ -run 'Backfill|Reconcile|Complete' -v`
Expected: PASS for all four tests. (Requires the Postgres from Task 2, Step 3. If the DB is unavailable, this task is **not** done — do not skip.)

- [ ] **Step 8: Commit**

```bash
git add internal/state/state.go internal/state/db/statepsql/backfills.go internal/state/db/statepsql/backfills_test.go internal/state/db/statefdb/backfills.go
git commit -m "feat(state): add guild-backfill marker methods (psql impl, fdb no-op)"
```

---

## Task 4: Handler — stamp markers, enrich `EventPayload`

**Files:**
- Modify: `handler/helpers.go:11-13` (struct) and `:23-28` (routing)
- Modify: `handler/guild.go:11-102` (`GuildCreate` signature + payload)
- Modify: `handler/member.go:10-22` (`MemberChunk` begin/complete)

**Interfaces:**
- Consumes: `discord.MemberChunk.ChunkIndex/ChunkCount` (Task 1); `db.BeginGuildBackfill/CompleteGuildBackfill` (Task 3).
- Produces: `handler.EventPayload{ GuildID int64; MemberCount int64; ReceivedMembers int; MemberIDs []int64 }`; `GuildCreate(ctx, d) (*EventPayload, error)`. Task 5 consumes `EventPayload` in the session.

- [ ] **Step 1: Expand `EventPayload` and update routing**

In `handler/helpers.go`, change the struct (lines 11-13):

```go
type EventPayload struct {
	GuildID         int64
	MemberCount     int64
	ReceivedMembers int
	MemberIDs       []int64
}
```

Then change the GUILD_CREATE / GUILD_UPDATE cases (lines 23-28) to use the new signature directly:

```go
	case "GUILD_CREATE":
		return c.GuildCreate(ctx, e.D)
	case "GUILD_UPDATE":
		return c.GuildCreate(ctx, e.D)
```

(GUILD_UPDATE routes through the same handler as before; the session only acts on the payload for GUILD_CREATE, so the extra fields are harmless on updates.)

- [ ] **Step 2: Change `GuildCreate` to return the enriched payload**

In `handler/guild.go`, change the signature and the return statements of `GuildCreate`. Header (line 11) becomes:

```go
func (c *Client) GuildCreate(ctx context.Context, d []byte) (*EventPayload, error) {
```

The decode-error return (lines 13-15) becomes:

```go
	gc, err := c.enc.DecodeGuildCreate(d)
	if err != nil {
		return nil, xerrors.Errorf("parse guild create: %w", err)
	}
```

Replace the trailing `guild := gc.ID` / final `return guild, err` with a built payload. Delete the `guild := gc.ID` line (line 17) and replace the final `err = eg.Wait(); return guild, err` (lines 100-101) with:

```go
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	memberIDs := make([]int64, 0, len(gc.Members))
	for id := range gc.Members {
		memberIDs = append(memberIDs, id)
	}

	return &EventPayload{
		GuildID:         gc.ID,
		MemberCount:     gc.MemberCount,
		ReceivedMembers: len(gc.Members),
		MemberIDs:       memberIDs,
	}, nil
```

(Every `eg.Go` closure references `gc.ID` directly, so removing the `guild` local is safe. `gc.Members` keys are the member IDs decoded from the GUILD_CREATE payload — no decoder change needed.)

- [ ] **Step 3: Build to confirm the handler package compiles**

Run: `go build ./handler/...`
Expected: no output (success). `HandleEvent` already returns `(*EventPayload, error)`, so the new `GuildCreate` signature matches the `return c.GuildCreate(...)` calls.

- [ ] **Step 4: Stamp begin/complete markers in `MemberChunk`**

In `handler/member.go`, replace the body of `MemberChunk` (lines 10-22) with:

```go
func (c *Client) MemberChunk(ctx context.Context, d []byte) error {
	mc, err := c.enc.DecodeMemberChunk(d)
	if err != nil {
		return err
	}

	err = c.db.SetGuildMembers(ctx, mc.GuildID, mc.Members)
	if err != nil {
		c.log.Error(ctx, "failed to set members", slog.Error(err))
	}

	if mc.ChunkIndex == 0 {
		if err := c.db.BeginGuildBackfill(ctx, mc.GuildID); err != nil {
			c.log.Error(ctx, "begin guild backfill", slog.Error(err), slog.F("guild", mc.GuildID))
		}
	}
	// ChunkCount > 0 guard: a payload missing the field decodes to 0 and can never stamp a marker.
	if mc.ChunkCount > 0 && mc.ChunkIndex == mc.ChunkCount-1 {
		if err := c.db.CompleteGuildBackfill(ctx, mc.GuildID); err != nil {
			c.log.Error(ctx, "complete guild backfill", slog.Error(err), slog.F("guild", mc.GuildID))
		}
	}

	return nil
}
```

(`slog` is already imported in `member.go`. Marker errors are logged, not returned — fail open, per the global constraints.)

- [ ] **Step 5: Build the handler package**

Run: `go build ./handler/...`
Expected: no output (success).

- [ ] **Step 6: Commit**

```bash
git add handler/helpers.go handler/guild.go handler/member.go
git commit -m "feat(handler): stamp backfill markers and enrich GUILD_CREATE payload"
```

---

## Task 5: Session — preload markers and gate RGM

**Files:**
- Create: `internal/gatewayws/backfill.go`
- Create: `internal/gatewayws/backfill_test.go`
- Modify: `internal/gatewayws/ws.go:38-90` (Session field)
- Modify: `internal/gatewayws/ws.go:320-325` (skip switch)
- Modify: `internal/gatewayws/ws.go:403-419` (READY preload)
- Modify: `internal/gatewayws/write.go:113` (use the const)

**Interfaces:**
- Consumes: `handler.EventPayload` (Task 4); `s.stateDB.GetGuildBackfillTimes` / `ReconcileGuildMembers` (Task 3); existing `s.stateDB state.DB`, `s.guilds map[int64]struct{}`, `s.requestGuildMembers`, `s.shouldProcessMembers`.
- Produces: `Session.backfilled map[int64]struct{}`; helpers `membersComplete`, `jitterFactor`, `isFresh`, `backfillStalenessWindow`, `LargeThreshold`.

- [ ] **Step 1: Write the failing helper unit tests**

Create `internal/gatewayws/backfill_test.go`:

```go
package gatewayws

import (
	"testing"

	"github.com/tatsuworks/gateway/handler"
)

func TestMembersComplete(t *testing.T) {
	cases := []struct {
		name string
		p    handler.EventPayload
		want bool
	}{
		{"small full", handler.EventPayload{MemberCount: 10, ReceivedMembers: 10}, true},
		{"small over-received", handler.EventPayload{MemberCount: 10, ReceivedMembers: 12}, true},
		{"small partial", handler.EventPayload{MemberCount: 10, ReceivedMembers: 4}, false},
		{"at threshold", handler.EventPayload{MemberCount: LargeThreshold, ReceivedMembers: LargeThreshold}, true},
		{"large guild", handler.EventPayload{MemberCount: LargeThreshold + 1, ReceivedMembers: LargeThreshold + 1}, false},
		{"zero count", handler.EventPayload{MemberCount: 0, ReceivedMembers: 0}, false},
	}
	for _, c := range cases {
		if got := membersComplete(&c.p, LargeThreshold); got != c.want {
			t.Errorf("%s: membersComplete = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestJitterFactorBounds(t *testing.T) {
	for _, id := range []int64{0, 1, 7, 173184118492889089, -5, 1 << 40} {
		f := jitterFactor(id)
		if f < 0.75 || f >= 1.25 {
			t.Errorf("jitterFactor(%d) = %v, want [0.75, 1.25)", id, f)
		}
		if jitterFactor(id) != f {
			t.Errorf("jitterFactor(%d) not deterministic", id)
		}
	}
}
```

- [ ] **Step 2: Run to confirm it fails**

Run: `go test ./internal/gatewayws/ -run 'MembersComplete|JitterFactor' -v`
Expected: FAIL — `membersComplete`, `jitterFactor`, `LargeThreshold` are undefined.

- [ ] **Step 3: Create the helpers**

Create `internal/gatewayws/backfill.go`:

```go
package gatewayws

import (
	"hash/fnv"
	"os"
	"strconv"
	"time"

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

// backfillStalenessWindow is the base marker lifetime, from
// BACKFILL_STALENESS_HOURS (default 24h).
func backfillStalenessWindow() time.Duration {
	if v, err := strconv.Atoi(os.Getenv("BACKFILL_STALENESS_HOURS")); err == nil && v > 0 {
		return time.Duration(v) * time.Hour
	}
	return 24 * time.Hour
}

// jitterFactor maps a guild ID deterministically into [0.75, 1.25) so markers
// stamped together (e.g. a cold deploy) expire spread over a ~12h band instead
// of all at once.
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
// staleness threshold.
func isFresh(guildID int64, backfilledAt time.Time, base time.Duration) bool {
	threshold := time.Duration(float64(base) * jitterFactor(guildID))
	return time.Since(backfilledAt) < threshold
}
```

- [ ] **Step 4: Run to confirm the helper tests pass**

Run: `go test ./internal/gatewayws/ -run 'MembersComplete|JitterFactor' -v`
Expected: PASS

- [ ] **Step 5: Add the `backfilled` field to `Session`**

In `internal/gatewayws/ws.go`, in the `Session` struct, change the `guilds` line (line 72) region to add the field:

```go
	guilds     map[int64]struct{}
	backfilled map[int64]struct{}
	curState   string
```

- [ ] **Step 6: Preload markers in the READY branch**

In `internal/gatewayws/ws.go`, in the `case "READY":` block, after the `for i := range guilds { s.guilds[i] = struct{}{} }` loop (around line 413) and before `s.sessID = sess`, insert:

```go
		s.backfilled = map[int64]struct{}{}
		ids := make([]int64, 0, len(s.guilds))
		for id := range s.guilds {
			ids = append(ids, id)
		}
		if times, terr := s.stateDB.GetGuildBackfillTimes(s.ctx, ids); terr != nil {
			s.log.Error(s.ctx, "preload guild backfill times", slog.Error(terr))
		} else {
			window := backfillStalenessWindow()
			for id, at := range times {
				if isFresh(id, at, window) {
					s.backfilled[id] = struct{}{}
				}
			}
		}
```

(Fail open: on error `s.backfilled` stays empty and every guild takes the RGM path. The resume path never enters this branch, so a resumed session has a nil `backfilled` map — handled in Step 7 with a nil-safe lookup.)

- [ ] **Step 7: Replace the unconditional RGM with the skip switch**

In `internal/gatewayws/ws.go`, replace the RGM block (lines 320-325):

```go
		// request guild members on GUILD_CREATE, gated by the backfill marker
		if s.shouldProcessMembers() && ev.T == "GUILD_CREATE" && evtPayload != nil && evtPayload.GuildID != 0 {
			s.curState = "maybe request guild members"
			s.maybeRequestGuildMembers(ctx, evtPayload)
		}
```

Then add the method to `internal/gatewayws/backfill.go`:

```go
// maybeRequestGuildMembers decides, per GUILD_CREATE, whether to skip RGM
// (fresh marker), reconcile a small guild straight from the payload roster, or
// fall back to RGM. Only the small-guild branch does DB work, and that at most
// once per guild per staleness window.
func (s *Session) maybeRequestGuildMembers(ctx context.Context, p *handler.EventPayload) {
	switch {
	case s.isBackfilled(p.GuildID):
		s.log.Debug(s.ctx, "skipping rgm: backfilled", slog.F("guild", p.GuildID))
	case membersComplete(p, LargeThreshold):
		if err := s.stateDB.ReconcileGuildMembers(ctx, p.GuildID, p.MemberIDs); err != nil {
			s.log.Error(s.ctx, "reconcile guild members", slog.Error(err), slog.F("guild", p.GuildID))
		}
	default:
		s.requestGuildMembers(p.GuildID)
	}
}

func (s *Session) isBackfilled(guildID int64) bool {
	_, ok := s.backfilled[guildID] // nil-map read is safe and returns false
	return ok
}
```

Add the needed imports to `internal/gatewayws/backfill.go`'s import block: `"context"`, `"cdr.dev/slog"`. (`handler` and `time` are already imported from Step 3.)

- [ ] **Step 8: Use the const in IDENTIFY**

In `internal/gatewayws/write.go`, change line 113 from `LargeThreshold: 250,` to:

```go
			LargeThreshold: LargeThreshold,
```

(The `Identify` struct field `LargeThreshold` is a different identifier from the package const; Go resolves the value expression to the const. This keeps the inline-member count and the small-guild predicate on the same number.)

- [ ] **Step 9: Build and run the full gatewayws test suite**

Run: `go build ./... && go test ./internal/gatewayws/ -v`
Expected: build succeeds; `MembersComplete`, `JitterFactor`, and the existing `intents_test.go` tests PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/gatewayws/backfill.go internal/gatewayws/backfill_test.go internal/gatewayws/ws.go internal/gatewayws/write.go
git commit -m "feat(gatewayws): gate RGM on backfill markers with jittered staleness"
```

---

## Task 6: Full verification

**Files:** none (verification + artifacts).

- [ ] **Step 1: Build everything**

Run: `go build ./...`
Expected: no output (success).

- [ ] **Step 2: Vet**

Run: `go vet ./...`
Expected: no output (or only pre-existing, unrelated warnings — compare against `git stash` baseline if unsure).

- [ ] **Step 3: Run the full unit suite (no-DB packages)**

Run: `go test ./discord/... ./internal/gatewayws/...`
Expected: PASS.

- [ ] **Step 4: Run the statepsql integration tests against a live DB**

Run: `go test ./internal/state/db/statepsql/ -run 'Backfill|Reconcile|Complete'`
Expected: PASS (requires the Postgres from Task 2).

- [ ] **Step 5: Record evidence**

Append a session entry to `agent-progress.md` (verified state, the exact commands run, and their PASS output) and add/flip a `feature_list.json` entry for `backfill-marker-rgm-skip` to `passing` with the evidence commands. Keep within the existing JSON shape.

- [ ] **Step 6: Commit the artifacts**

```bash
git add agent-progress.md feature_list.json
git commit -m "docs: record backfill-marker-rgm-skip verification"
```

---

## Self-Review

**Spec coverage** (against `docs/superpowers/specs/2026-06-12-backfill-marker-rgm-skip-design.md`):
- Schema (`guild_backfills`, UNLOGGED, PK) → Task 2. ✓
- `chunk_index`/`chunk_count` in both decoders → Task 1. ✓
- `BeginGuildBackfill` on chunk 0, `CompleteGuildBackfill` on last chunk with `ChunkCount>0` guard → Task 4. ✓
- Reconciliation delete (`last_updated < started_at`) + `backfilled_at` stamp → Task 3 (`CompleteGuildBackfill`). ✓
- Four `state.DB` methods; statepsql real, statefdb no-op → Task 3. ✓
- Bulk read excluding NULL `backfilled_at` via `ANY($1)` → Task 3 (`GetGuildBackfillTimes`) + test. ✓
- Session preload building the fresh-marker set → Task 5, Step 6. ✓
- `BACKFILL_STALENESS_HOURS` default 24 + per-guild jitter [0.75,1.25] → Task 5 (`backfillStalenessWindow`, `jitterFactor`). ✓
- Skip switch (backfilled / small-guild reconcile / default RGM) → Task 5, Step 7. ✓
- Small-guild payload reconcile (`ReconcileGuildMembers`, exact `!= ALL`) → Task 3 + Task 5. ✓
- `EventPayload` carries `MemberCount`/`ReceivedMembers`/`MemberIDs` → Task 4. ✓
- GUILD_UPDATE no longer does RGM work → preserved (session acts only on `ev.T == "GUILD_CREATE"`). ✓
- Error handling table (fail open everywhere) → marker errors logged not returned (Tasks 4, 5); tx rollback (Task 3). ✓
- Testing section (decoder tests, statepsql tests, predicate + jitter unit tests) → Tasks 1, 3, 5. ✓

**Deviations from the design doc, by design (baseline = master):**
- No `GuildHasMembers` removal and no 5-value→2-value `GuildCreate` change — neither exists on this branch; we build the end state directly (see Global Constraints).
- The decoder `default:` empty→`readTerm()` fix (Task 1) is an addition the design didn't call out, required because this branch's `DecodeMemberChunk` never consumed unknown keys; needed for correct chunk-field extraction.

**Placeholder scan:** none — every code step shows complete code.

**Type consistency:** `EventPayload` fields (`GuildID int64`, `MemberCount int64`, `ReceivedMembers int`, `MemberIDs []int64`) are used identically in Task 4 (producer) and Task 5 (`membersComplete`, `maybeRequestGuildMembers`). The four `state.DB` signatures match across interface (Task 3 Step 1), psql impl, fdb impl, and call sites (Tasks 4, 5). `LargeThreshold` is one package-level `const` referenced by `membersComplete`, the test, and `writeIdentify`.
