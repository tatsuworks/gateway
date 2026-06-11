# Faster Gateway Mass Reconnect

Status: Design

## Problem

When many shards must re-IDENTIFY simultaneously (Discord-side outage long enough to invalidate sessions, or any scenario that pushes most shards off the resume path), wall-clock time to fully re-identify ~720 shards is dominated by an internal serialization, not Discord's identify rate limit.

Today, `Open()` on a fresh identify acquires an etcd mutex bucketed by `shardID % 16`, then holds that mutex from before the WebSocket dial through 70s after READY (10s `IdentifyWaitTime` + 60s `IdentifyStabilizeTime` when the GuildMembers intent is on). With 720 shards across 16 buckets, that's ~45 serial identifies per bucket × 70s ≈ **52 minutes worst case**.

Discord's identify rate limit (5s per `max_concurrency` slot) would otherwise allow ~7.5 minutes for the same workload at 16 concurrent buckets.

The internal 60s `IdentifyStabilizeTime` exists to "let the DB catch up" while a shard's GUILD_CREATE → `Request Guild Members` → `GUILD_MEMBER_CHUNK` backfill drains. It is currently coupled to identify pacing and to the `IntentGuildMembers` flag, even though those are independent concerns. The `state.DB` PostgreSQL backend already has a sharded, deduping batcher (`internal/state/db/statepsql/batcher.go`) that absorbs hot-key bursts; whether the 60s wait is still load-bearing has not been measured. There is also no full-resync mechanism today, so backfill on every reconnect is the only thing keeping member rosters from drifting.

The "fast intents" deployment mode reconnects in ~15 minutes precisely because it sets `hasGuildMembersIntent = false`, which makes `calcIdentifyWait()` skip the 60s wait. The ops cost is dropping intents that the consumers actually want — i.e., it solves reconnect time by giving up correctness.

## Goals

- Reduce wall-clock time for an identify-storm reconnect of ~720 shards from ~50 minutes toward Discord's identify-throughput floor (~7–8 minutes at the current 16 buckets).
- Keep Discord's 5s-per-bucket identify pacing honored.
- Keep the existing safety property that DB writes don't get hammered by simultaneous member-chunk bursts from many shards — but make that a separate, measurable mechanism instead of a fixed wall inside the identify lock.
- Bound member-roster drift without forcing a full backfill on every reconnect.
- No behavioral change in the resume path. It already skips the lock; leave it alone.

## Non-Goals

- Replacing the etcd mutex with an in-process semaphore. Single-pod-per-shard-range makes it feasible, but it's a separate cleanup.
- Auto-discovering Discord `max_concurrency` from `/gateway/bot`. The 16-bucket constant is hardcoded as `s.shardID % 16`; making it dynamic is orthogonal mechanical work.
- Touching the manager-loop 1s sleep between reconnect attempts.
- Changing persistence (sessID/seq/resumeURL) to keep more shards on the resume path. Different problem (transient-blip scenario).
- Two-phase identify (FastIntents first, full intents second). Doubles identify count and creates an event-loss window; rejected.

## Architecture

### Today

```
Open()
  loadSeq/sessID/resumeURL
  initEtcd (lease, 70s TTL)
  acquireIdentifyLock (mod-16 bucket)        ─┐ lock held
    dial WS                                   │ throughout
    readHello                                 │ ~70s
    writeIdentify                             │
    READY received                            │
    goroutine sleeps 70s → releaseIdentifyLock┘
    handle events…
```

The mutex covers the dial, hello exchange, identify, AND the 60s post-READY stabilize wait. Whatever is the slowest step inside that window holds up every other shard in the same bucket.

### Proposed

```
Open()
  loadSeq/sessID/resumeURL
  initEtcd
  dial WS                          ─┐ MOVED outside lock (Phase 1, Approach 2)
  readHello                         │
  acquireIdentifyLock (mod-N bucket)─┐ lock held only for
    writeIdentify                    │ Discord's pacing window
    wait for READY (with grace)      │ (~5–10s)
  releaseIdentifyLock               ─┘
  if shouldRequestMembers(guild):     ← per-guild decision, Phase 3a
     acquire stabilize semaphore     ─┐ separate gate, Phase 1
     wait for backfill drain          │ (chunk-count signal, Phase 2)
     release stabilize semaphore      │
  handle events…
```

Two new mechanisms replace the single overloaded mutex:

**Identify lock** — same etcd mutex (`/gateway/identify/<bucket>`), but holds only across `writeIdentify` → READY-or-pacing-elapsed. Released as soon as the shard is past the rate-limit-relevant step.

**DB-stabilization gate** — separate in-process weighted semaphore (no etcd involvement, single-pod confirmed). Capacity defaults to 1 (matches current behavior — one stabilizing shard at a time). Knob: `IDENTIFY_STABILIZE_CONCURRENCY`. Wait policy in Phase 1 is a fixed `IDENTIFY_STABILIZE_SECONDS` defaulting to 60s; replaced in Phase 2 by chunk-drain signal.

### Conditional backfill

`shouldRequestMembers(guild)` in Phase 3a replaces today's blanket `shouldProcessMembers()` check at GUILD_CREATE. Decision rule:

```
should_request_members(guild) =
    !skipMemberRequest
    && hasGuildMembersIntent
    && (
        last_backfilled_at IS NULL                          // cold guild
        OR now - last_backfilled_at > STALENESS_THRESHOLD   // stale
        OR abs(discord_member_count - cached_count)
              / discord_member_count > DIVERGENCE_THRESHOLD // divergent
    )
```

> **Implementation status (this PR):** the `last_backfilled_at` column and the
> staleness/divergence arms above are not yet built. The first increment ships
> the cold-guild arm only, using a `GuildHasMembers(guild)` `EXISTS` check
> (`SELECT EXISTS(SELECT 1 FROM members WHERE guild_id = $1)`) as a proxy for
> `last_backfilled_at IS NULL`: if any members are already cached we skip RGM on
> this connect cycle, otherwise we request. The schema change and the
> stale/divergent arms land in a follow-up.

Triggers:

| Trigger | Why | Default |
|---|---|---|
| Cold | First contact after fresh DB; no cached roster | always |
| Stale | Bound roster drift over time | 24h |
| Divergent | Catch silent drift between scheduled refreshes | 5% |

Backfill completion: when `chunk_index == chunk_count - 1` for a guild's `GUILD_MEMBER_CHUNK`, write `last_backfilled_at = now` to that guild's row. Per-shard in-memory map tracks in-flight backfills (`guild_id → expected_chunks`).

### Background re-sync sweep (Phase 3b)

One goroutine per process (in the manager, not per shard) walks `state.DB` periodically:

- Tick every `BACKFILL_SWEEP_INTERVAL` (default 1h).
- Find guilds where `last_backfilled_at < now - STALENESS_THRESHOLD` AND not currently in-flight.
- For each, look up the shard that owns the guild (`(guildID >> 22) % shardCount`) and route a `Request Guild Members` op through that shard's session.
- Pace at `BACKFILL_REQUESTS_PER_SECOND` (default 2/s) globally to avoid a thundering herd on Discord and the DB batcher.

The sweep decouples staleness-driven re-sync from the connect path entirely. After Phase 3b lands, the connect-path conditional check in Phase 3a only catches **cold** and **divergent** guilds; routine staleness is the sweep's job.

### Failure handling

- **Pre-lock dial failure** — discard the connection, no lock held, retry from the top. (Today's flow holds the lock during a failed dial too; new flow is strictly cheaper for failures.)
- **Lock acquired, READY never arrives** — time out at `IdentifyWaitTime + grace` (configurable, default 5s grace), release lock, fall through to retry. Avoids one bad shard wedging a bucket for the full etcd lease TTL.
- **Stabilize semaphore acquired, drain never completes** — time out at `IDENTIFY_STABILIZE_SECONDS_MAX` (default 120s, 2× current 60s ceiling), release semaphore, log a warning, continue. We never block forever on the stabilize gate.
- **Resume path** — unchanged. Skips both the lock and the stabilize gate.
- **Sweep request to a disconnected shard** — drop silently; sweep will pick the guild up next tick once the shard reconnects.

### Components and ownership

| Component | Lives in | Responsibility |
|---|---|---|
| Identify lock (etcd mutex) | `internal/gatewayws` | Discord identify pacing, mod-N bucket |
| Stabilize semaphore | `internal/manager` (shared across shards) | Cap concurrent backfill bursts hitting `state.DB` |
| Backfill metadata column | `state.DB` schema (psql + fdb) | Per-guild `last_backfilled_at` |
| Conditional check | `gatewayws.Session` GUILD_CREATE branch in `Open()` | Decide whether to call `requestGuildMembers`; reads `last_backfilled_at` and `member_count` via `state.DB` |
| Chunk-drain tracker | `gatewayws.Session` | Map of in-flight guild → expected/received chunks |
| Background sweep | `internal/manager` | Periodic staleness scan, route to shards |

Schema delta:

```sql
ALTER TABLE guilds ADD COLUMN last_backfilled_at timestamp NULL;
CREATE INDEX guilds_last_backfilled_at ON guilds (last_backfilled_at)
    WHERE last_backfilled_at IS NOT NULL;
```

The partial index supports the sweep's "find stale guilds" query without bloating writes for cold guilds.

The corresponding FoundationDB representation stores the timestamp in the same guild key's value blob (or as a sibling key under the guild prefix — implementation detail for the FDB backend).

## Observability

New metrics needed before tuning the knobs:

- `gateway_identify_lock_wait_seconds` — time from `acquireIdentifyLock` call to acquired
- `gateway_identify_lock_held_seconds` — time lock is actually held (target: shrinks dramatically)
- `gateway_stabilize_wait_seconds` — time spent in stabilize semaphore
- `gateway_backfill_chunks_received_total{guild_id}` — counter
- `gateway_backfill_drain_seconds` — time from `requestGuildMembers` send to last chunk for a guild
- `gateway_backfill_skipped_total{reason}` — `cold|stale|divergent|fresh|disabled`
- `gateway_sweep_guilds_resynced_total`
- `gateway_sweep_queue_depth`
- `state_db_batcher_queue_depth{event_type}` — already exists or can be added; needed to validate the batcher absorbs the load

## Phasing

**Phase 1 — Lock shrink + decoupled stabilize gate.**

Move dial + hello before lock acquisition. Reduce lock scope to identify-send through READY-or-pacing-grace. Add stabilize semaphore in `internal/manager` with `IDENTIFY_STABILIZE_CONCURRENCY` (default 1) and `IDENTIFY_STABILIZE_SECONDS` (default 60). Add the metrics above. Behavior is identical for `SKIP_MEMBER_REQUEST=true` deploys; mass identify becomes ~7× faster for prod's `SKIP_MEMBER_REQUEST=false` deploy because the stabilize wait runs in parallel with the next shard's identify rather than serializing it.

**Phase 2 — Drain-based stabilize signal.**

Replace the fixed `IDENTIFY_STABILIZE_SECONDS` with the chunk-drain tracker. Stabilize is "done" when this shard's outstanding chunk count is zero AND the batcher queue depth is below a threshold. Keeps the upper-bound timeout from Phase 1 as a safety. After this lands, the stabilize semaphore caps concurrent draining shards (still default 1) but the per-shard wait is "as long as actually needed."

**Phase 3a — Conditional backfill at GUILD_CREATE.**

Add `last_backfilled_at` column. Implement `shouldRequestMembers(guild)` with cold/stale/divergent triggers. Introduce env knobs: `BACKFILL_STALENESS_HOURS` (default 24), `BACKFILL_DIVERGENCE_RATIO` (default 0.05). Steady-state mass identify on an already-populated DB now skips most member requests entirely — reconnect approaches the Discord-floor lower bound.

**Phase 3b — Background re-sync sweep.**

Implement the manager-level goroutine. Knobs: `BACKFILL_SWEEP_INTERVAL` (default 1h), `BACKFILL_REQUESTS_PER_SECOND` (default 2). Sweep takes over staleness-driven re-syncs; Phase 3a's connect-path check then only fires for cold + divergent guilds.

Each phase is independently shippable. Phase 1 is the structural change; Phases 2 and 3 progressively reduce the stabilize wait toward zero in steady state.

## Expected impact

| Phase | Mass identify of 720 shards | Notes |
|---|---|---|
| Today | ~50 min | 45 × 70s, lock dominates |
| Phase 1, default config | ~45 min | Stabilize runs in parallel with subsequent identifies, but with `IDENTIFY_STABILIZE_CONCURRENCY=1` the stabilize gate becomes the new serial bottleneck (~45 × 60s). Phase 1 alone is mostly about unlocking the tuning surface, not shipping a smaller wall. |
| Phase 1 + concurrency raised | ~10–15 min | Once metrics confirm the batcher absorbs N concurrent draining shards, raise `IDENTIFY_STABILIZE_CONCURRENCY`. With N=4 → ~12 min; N=8 → ~7.5 min (Discord-floor). |
| Phase 2 | ~7.5–10 min typical | Drain signal beats fixed 60s for most guilds; concurrency knob still applies |
| Phase 3a + 3b (steady state) | ~7.5 min (Discord floor) | Most guilds skip backfill entirely; sweep handles staleness out-of-band |

## Open questions

None blocking. Items deferred to implementation:

- Exact metric names + label cardinality (member chunk metrics keyed by guild_id may need sampling).
- FoundationDB representation of `last_backfilled_at` (existing guild value vs sibling key).
- Whether to expose stabilize-semaphore tuning via the existing gRPC management API for live ops.
