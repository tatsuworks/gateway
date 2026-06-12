# Backfill-Completion Marker for RGM Skip

Status: Design (approved, supersedes the `GuildHasMembers` mechanism on `feat/gate-rgm-on-reidentify`)

## Problem

The current branch skips `Request Guild Members` (RGM) on GUILD_CREATE when
`GuildHasMembers` — `EXISTS(SELECT 1 FROM members WHERE guild_id=$1)` — returns
true. That mechanism has four defects:

1. **Wrong signal.** `GuildCreate` writes the payload's partial member slice
   (up to ~250 online members for large guilds) before RGM runs, so EXISTS is
   true after the *first contact* with a guild, regardless of whether backfill
   completed. If the first backfill is interrupted (disconnect mid-chunk,
   restart, or `requestGuildMembers` silently dropping the op on a full write
   channel), every future session skips RGM and the roster stays partial
   forever.
2. **No drift bound.** Completed backfills are never refreshed; missing
   members are never re-added. Worse, *no* backfill — today's, the branch's,
   or a naive re-backfill — removes departed members: chunk writes are pure
   upserts (`processMemberBatch`, `INSERT ... ON CONFLICT DO UPDATE`), and
   the only delete path is the live `GUILD_MEMBER_REMOVE` event, which is
   missed whenever the gateway is disconnected. Ghost members accumulate
   forever.
3. **Hot-path DB probe.** The EXISTS query runs synchronously in the session
   read loop for every GUILD_CREATE — thousands per shard × 720 shards aimed
   at PSQL exactly during a reconnect storm — and also fires (result
   discarded) on every steady-state GUILD_UPDATE, which routes through the
   same handler.
4. **Plumbing.** `GuildCreate` returns 5 positional values, `EventPayload`
   grew 3 fields, the probe's error is silently swallowed, and the skip
   decision is split across handler and session.

## Design

Replace the per-event EXISTS probe with a **completion marker written once
when a backfill finishes**, read **once per connect in bulk**.

### Schema

`init.sql` (with matching drop in `down.sql`):

```sql
CREATE UNLOGGED TABLE IF NOT EXISTS guild_backfills (
    "guild_id"   int8 NOT NULL,
    "started_at" timestamp NOT NULL DEFAULT now(),
    "backfilled_at" timestamp NULL,
    PRIMARY KEY ("guild_id")
);
```

`started_at` records when the current backfill's first chunk arrived (DB
clock); `backfilled_at` is set only when the final chunk lands. Both
timestamps come from the same Postgres clock as the `members.last_updated`
trigger, which the reconciliation delete below compares against — app-clock
skew can never cause a wrong delete.

Deliberately `UNLOGGED`, matching `members`: a PSQL crash that wipes the cache
tables also wipes the markers, so every guild reads as cold and re-backfills.
Markers can never outlive the member data they vouch for.

### Completion detection

Discord includes `chunk_index` and `chunk_count` in every
`GUILD_MEMBER_CHUNK`; both decoders currently drop them.

- Add `ChunkIndex, ChunkCount int` to `discord.MemberChunk`.
- Extract both fields in `discordetf.DecodeMemberChunk` and
  `discordjson.DecodeMemberChunk`.
- In `handler.MemberChunk`, after `SetGuildMembers` succeeds:
  - `ChunkIndex == 0` → `db.BeginGuildBackfill(ctx, guildID)`: upsert the
    `guild_backfills` row with `started_at = now()`, leaving any prior
    `backfilled_at` untouched (an in-flight re-backfill doesn't invalidate
    the previous completed one).
  - `ChunkCount > 0 && ChunkIndex == ChunkCount-1` →
    `db.CompleteGuildBackfill(ctx, guildID)`. The `ChunkCount > 0` guard
    means a payload missing the field can never stamp a marker.

### Reconciliation delete (ghost-member cleanup)

`CompleteGuildBackfill` runs one transaction:

```sql
DELETE FROM members
WHERE guild_id = $1
  AND last_updated < (SELECT started_at FROM guild_backfills WHERE guild_id = $1);
UPDATE guild_backfills SET backfilled_at = now() WHERE guild_id = $1;
```

A completed backfill upserts every current member, and the existing
`update_members_last_updated` trigger bumps `last_updated` on each. Anyone
the full roster didn't touch left the guild — delete them. Members updated
by live events during the drain have fresh `last_updated` and survive.

Interaction with the member batcher is safe by construction: if a chunk's
upserts are still queued when the delete runs, the delete removes the stale
row first and the queued `INSERT ... ON CONFLICT` re-creates it — the end
state is correct either way. This is the only mechanism in the system that
removes members who departed while the gateway was disconnected, which is
itself an argument for backfills that recur on a staleness bound rather
than being skipped forever.

**Cost.** The delete scans the guild's slice of the `(guild_id, user_id)`
PK and checks `last_updated` per row: sub-ms at 250 members, tens of ms at
100k, ~1–2s for a 1M-member guild. It runs only when a backfill completes —
immediately after upserting the same rows, so the scan hits warm cache and
is a fraction of the write work just done — at most once per guild per
staleness window, and never during reconnects that skip RGM. MVCC means the
scan blocks no readers/writers; row locks land only on deleted ghosts; the
unlogged table generates no WAL. Deliberately **no** `(guild_id,
last_updated)` index: it would tax every write on a hot table to speed an
infrequent, already-cheap operation. `MemberChunk` runs synchronously in the
read loop, so a mega-guild delete stalls that shard ~1–2s once a day —
consistent with existing `GuildCreate` behavior; if it matters in practice,
`CompleteGuildBackfill` can safely move to a goroutine (concurrent
member-add survives via fresh `last_updated`; concurrent member-remove is
idempotent with the delete).

### state.DB interface

Remove `GuildHasMembers`. Add:

```go
BeginGuildBackfill(ctx context.Context, guild int64) error
CompleteGuildBackfill(ctx context.Context, guild int64) error
ReconcileGuildMembers(ctx context.Context, guild int64, roster []int64) error
GetGuildBackfillTimes(ctx context.Context, guilds []int64) (map[int64]time.Time, error)
```

- **statepsql:** as described above; bulk read is
  `SELECT guild_id, backfilled_at FROM guild_backfills WHERE guild_id =
  ANY($1) AND backfilled_at IS NOT NULL` (PK index lookups via
  `pq.Int64Array`); the session applies the per-guild jittered staleness
  threshold to the returned times.
- **statefdb:** all three writers return nil; `GetGuildBackfillTimes`
  returns nil. FDB deployments always take the RGM slow path (and get no
  reconciliation delete), per the CLAUDE.md rule that cache-dependent
  optimizations target PSQL only.

### Session preload

In the READY branch of `handleInternalEvent` (after `s.guilds` is built), one
call:

```go
times := GetGuildBackfillTimes(ctx, keys(s.guilds))
s.backfilled = guilds whose backfilled_at is newer than staleness × jitter(guild_id)
```

- `BACKFILL_STALENESS_HOURS` env knob, default **24**.
- **Per-guild expiry jitter:** markers stamped together (e.g., one cold
  deploy) would otherwise all expire together, recreating a full backfill
  storm at the first identify after the 24h cliff. The effective threshold
  is `staleness × jitter(guild_id)` with `jitter ∈ [0.75, 1.25]` derived
  from a hash of the guild ID — deterministic, so a guild's expiry is
  stable across shards and restarts. The bulk query returns
  `(guild_id, backfilled_at)` and the session applies the per-guild
  threshold in Go. Expiries spread over a ~12h band and self-desynchronize
  after the first re-backfill cycle. (Jitter cannot help when uptime
  exceeds the max window ~30h — every marker is stale by then; that case
  is owned by the Phase 3b background sweep in the parent design doc,
  which keeps markers perpetually fresh while connected and stays out of
  scope here.)
- On error: log, leave the set empty — everything falls back to RGM (fail
  open).
- Resume path untouched: no preload runs, and a GUILD_CREATE for a
  newly-joined guild is not in the set, so it correctly triggers RGM.

### Skip decision (ws.go)

The *decision* is pure in-memory (marker set + payload counts); only the
stale-small-guild branch performs DB work, at most once per guild per
staleness window:

```go
switch {
case s.isBackfilled(guildID):
    // marker fresh within jittered staleness window — nothing to do
case payload.MembersComplete(LargeThreshold):
    // small guild, full roster in the GUILD_CREATE payload:
    // reconcile + stamp directly from the payload, no RGM needed
    db.ReconcileGuildMembers(ctx, guildID, payload.MemberIDs)
default:
    s.requestGuildMembers(guildID)
}
```

The small-guild completeness check keeps the branch's existing (corrected)
semantics: `0 < MemberCount <= LargeThreshold && ReceivedMembers >=
MemberCount`.

### Small guilds: reconcile from the payload

Small guilds never RGM, so chunks — and the chunk-driven reconciliation
delete — never run for them. But a payload-complete GUILD_CREATE *is* a
completed backfill: the full roster is in hand. When such a guild's marker
is stale or cold, `ReconcileGuildMembers` runs one transaction:

```sql
DELETE FROM members WHERE guild_id = $1 AND user_id != ALL($2);
INSERT INTO guild_backfills (guild_id, started_at, backfilled_at)
VALUES ($1, now(), now())
ON CONFLICT (guild_id) DO UPDATE SET started_at = now(), backfilled_at = now();
```

The delete is exact (roster IDs, no timestamps — immune to batcher
ordering, since queued upserts only contain IDs in the roster) and tiny
(≤ `LargeThreshold` rows via PK). Fresh-marker small guilds hit the first
switch case and do zero DB work, so reconnect storms stay cheap; stale ones
pay one small transaction per staleness window, paced by the same 24h +
jitter as large guilds. `EventPayload` carries `MemberIDs` from the
GUILD_CREATE decode to make the roster available at the decision point.

### Handler cleanup

`GuildCreate` drops the DB probe and the 5-value return; it returns
`(*EventPayload, error)` where `EventPayload` carries `GuildID`,
`MemberCount`, `ReceivedMembers`, and `MemberIDs` (the decoded member IDs,
already available from the existing GUILD_CREATE member decode). GUILD_UPDATE
(same handler) no longer does any RGM-related work.

## Error handling

| Failure | Behavior |
|---|---|
| Begin/Complete write fails | Logged; guild re-backfills next connect (fail open) |
| Complete tx fails mid-way | Transaction rolls back: no partial delete, no marker; re-backfills next connect |
| Preload query fails | Empty set; all guilds RGM — today's behavior |
| Backfill interrupted | Final chunk never arrives → `backfilled_at` stays old/NULL → self-heals next connect |
| Concurrent RGMs for one guild | Both streams carry the full roster; interleaved begin/complete still converge — every current member gets upserted after the later `started_at`, so the delete only removes true ghosts |
| PSQL crash wipes unlogged tables | Markers wiped with members; full re-backfill |

## Testing

- Decoder tests for `chunk_index`/`chunk_count` in both `discordetf` and
  `discordjson`, following existing test patterns.
- `statepsql` tests for `BeginGuildBackfill` / `CompleteGuildBackfill` /
  `GetGuildBackfillTimes`: upsert idempotence, NULL `backfilled_at`
  excluded, ANY-list filtering, begin-preserves-prior-`backfilled_at`, and
  the reconciliation delete (member untouched since `started_at` is
  removed; member upserted during the drain survives).
- `statepsql` test for `ReconcileGuildMembers`: member absent from the
  roster is deleted, members in the roster survive, marker stamped.
- Unit tests for the small-guild completeness predicate and the per-guild
  jittered staleness threshold (deterministic, bounded to [0.75, 1.25]).

## Expected impact vs the current branch

- Reconnect-storm DB reads: ~one EXISTS per guild (millions) → one query per
  shard (720 total).
- Steady-state GUILD_UPDATE query: eliminated.
- Partial backfills: self-heal instead of permanent.
- Roster drift: bounded at ~24h (jittered) in **both directions** for
  guilds of every size — missing members re-added by the next
  backfill/payload, departed members removed by the chunk-driven
  reconciliation delete (large guilds) or the exact payload reconcile
  (small guilds). Ghost removal is new capability: no prior version of the
  system ever deleted members who left while the gateway was disconnected.

## Out of scope

- Identify-lock shrink and stabilize semaphore (Phase 1 of
  `2026-04-30-faster-mass-reconnect-design.md`).
- Divergence trigger and background re-sync sweep (Phases 3a-divergent / 3b).
