# Faster Gateway Mass Reconnect

Status: Phase 1 + Phase 2 + Phase 3 shipped

## Problem

When many shards must re-IDENTIFY simultaneously (Discord-side outage long enough to invalidate sessions, or any scenario that pushes most shards off the resume path), wall-clock time to fully re-identify ~1024 shards is dominated by an internal serialization, not Discord's identify rate limit.

Today, `Open()` on a fresh identify acquires an etcd mutex bucketed by `shardID % 16`, then holds that mutex from before the WebSocket dial through 70s after READY (10s `IdentifyWaitTime` + 60s `IdentifyStabilizeTime` when the GuildMembers intent is on). With 1024 shards across 16 buckets, that's 64 serial identifies per bucket × 70s ≈ **75 minutes worst case**.

Discord's identify rate limit (5s per `max_concurrency` slot) would otherwise allow ~5–11 minutes for the same workload at 16 concurrent buckets, depending on pacing margin.

The internal 60s `IdentifyStabilizeTime` exists to "let the DB catch up" while a shard's GUILD_CREATE → `Request Guild Members` → `GUILD_MEMBER_CHUNK` backfill drains. It is currently coupled to identify pacing and to the `IntentGuildMembers` flag, even though those are independent concerns. The `state.DB` PostgreSQL backend already has a sharded, deduping batcher (`internal/state/db/statepsql/batcher.go`) that absorbs hot-key bursts; whether the 60s wait is still load-bearing has not been measured. There is also no full-resync mechanism today, so backfill on every reconnect is the only thing keeping member rosters from drifting.

The "fast intents" deployment mode reconnects in ~15 minutes precisely because it sets `hasGuildMembersIntent = false`, which makes `calcIdentifyWait()` skip the 60s wait. The ops cost is dropping intents that the consumers actually want — i.e., it solves reconnect time by giving up correctness.

## Goals

- Reduce wall-clock time for an identify-storm reconnect of ~1024 shards from ~75 minutes toward Discord's identify-throughput floor (~5–11 minutes at the current 16 buckets).
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

Per-guild metadata (`last_backfilled_at` columns, per-row staleness timestamps) is rejected: at Tatsu's scale (millions of guilds across 1024 shards) the write amplification on every chunk-drain, the cost of finding stale rows, and the storage overhead are not worth the per-guild precision. Two complementary signals are used instead, neither of which requires per-guild metadata.

**Phase 3a — connect-path divergence check.** At GUILD_CREATE, compare Discord's `member_count` against the cached count for the guild. Request members only if the divergence exceeds `BACKFILL_DIVERGENCE_RATIO` (default 0.05).

```
should_request_members(guild) =
    !skipMemberRequest
    && hasGuildMembersIntent
    && (
        cached_count == 0                                     // cold guild
        OR abs(discord_member_count - cached_count)
              / discord_member_count > DIVERGENCE_THRESHOLD   // divergent
    )
```

`cached_count == 0` cleanly subsumes the "cold" case: a fresh DB has no members for the guild, divergence is 100%, request fires. No new schema. The cached count is computed from `state.DB` (existing GetGuildCount-style read for the guild's member rows, or a maintained counter if cheaper).

**Phase 3b — bounded-rate cursor sweep.** One goroutine per process pages through guilds and dispatches Request Guild Members ops at a globally rate-limited pace. Crucially, the sweep never loads more than `BACKFILL_SWEEP_BATCH` (default 200) guild IDs into memory at a time.

```
loop:
  1. Read up to BATCH guild IDs from state.DB:
       SELECT id FROM guilds WHERE id > cursor ORDER BY id LIMIT BATCH
     (rides the guilds PK index — cheap range scan, no full-table load)
  2. For each id:
       owner_shard = (id >> 22) % shard_count
       if owner_shard outside this process's shard range: skip
       if shard session offline: skip (next sweep picks it up)
       send Request Guild Members via that shard's session
       wait BACKFILL_REQUESTS_PER_SECOND throttle
  3. cursor = last id from query result
     if rows < BATCH: cursor = 0  (wrap around)
  4. persist cursor (1 row total per process), sleep BACKFILL_SWEEP_BATCH_INTERVAL
```

Properties:

- **Memory bound:** at most `BACKFILL_SWEEP_BATCH` guild IDs in memory at a time. Never enumerates the full guild set.
- **Storage:** one cursor per process, persisted in the existing `shards` table or a dedicated tiny `sweep_state` row. No per-guild metadata.
- **Predictable drift bound:** `full_sweep_period = guild_count / BACKFILL_REQUESTS_PER_SECOND`. For 1M guilds at 12 req/s → ~24h per full sweep. Tunable per ops policy.
- **Index cost:** the range-scan query uses the guilds PK; no new indexes needed.
- **Shard ownership filter is in-memory** on the small batch — does not push a `(id >> 22) % shard_count` predicate to the database.
- **Cursor restart resilience:** persisted cursor survives process restarts; no bias toward low-id guilds after redeploy.

The two phases are complementary. Phase 3a catches the urgent "we just reconnected, member_count is way off" case immediately. Phase 3b bounds steady-state drift for guilds that look fine at GUILD_CREATE but quietly accumulate ghosts over time.

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
| Divergence check | `gatewayws.Session` GUILD_CREATE branch in `Open()` | Compare Discord `member_count` vs cached count; decide whether to call `requestGuildMembers` |
| Chunk-drain tracker | `gatewayws.Session` | Map of in-flight guild → expected/received chunks |
| Sweep cursor | `internal/manager` (one row in state.DB or dedicated table per process) | Cursor position for the bounded-rate sweep |
| Background sweep | `internal/manager` | Pages guilds via PK range scan, rate-limited member requests |

No per-guild schema delta. No new indexes. The sweep relies on the existing guilds PK; the divergence check relies on the existing member count in `state.DB`.

If the cached member count is not cheap to compute on demand (i.e., counting member rows per guild is too heavy at GUILD_CREATE time), introduce a single `member_count` column on the guilds row maintained as a counter via the existing batcher. Cheaper than a timestamp + index, and useful beyond this feature. Decision deferred to Phase 3a implementation based on measured cost of the existing path.

## Observability

New metrics needed before tuning the knobs:

- `gateway_identify_lock_wait_seconds` — time from `acquireIdentifyLock` call to acquired
- `gateway_identify_lock_held_seconds` — time lock is actually held (target: shrinks dramatically)
- `gateway_stabilize_wait_seconds` — time spent in stabilize semaphore
- `gateway_backfill_chunks_received_total` — counter (no per-guild label)
- `gateway_backfill_drain_seconds` — time from `requestGuildMembers` send to last chunk for a guild
- `gateway_backfill_skipped_total{reason}` — `divergent_below_threshold|disabled|shard_offline`
- `gateway_sweep_guilds_dispatched_total`
- `gateway_sweep_cursor_position` — for tracking sweep progress
- `gateway_sweep_full_cycle_seconds` — time from cursor=0 back to cursor=0
- `state_db_batcher_queue_depth{event_type}` — already exists or can be added; needed to validate the batcher absorbs the load

## Phasing

**Phase 1 — Lock shrink + decoupled stabilize gate.**

Move dial + hello before lock acquisition. Reduce lock scope to identify-send through READY-or-pacing-grace. Add stabilize semaphore in `internal/manager` with `IDENTIFY_STABILIZE_CONCURRENCY` (default 1) and `IDENTIFY_STABILIZE_SECONDS` (default 60). Add the metrics above. Behavior is identical for `SKIP_MEMBER_REQUEST=true` deploys; mass identify becomes ~7× faster for prod's `SKIP_MEMBER_REQUEST=false` deploy because the stabilize wait runs in parallel with the next shard's identify rather than serializing it.

**Phase 2 — Drain-based stabilize signal.**

Replace the fixed `IDENTIFY_STABILIZE_SECONDS` with the chunk-drain tracker. Stabilize is "done" when this shard's outstanding chunk count is zero AND the batcher queue depth is below a threshold. Keeps the upper-bound timeout from Phase 1 as a safety. After this lands, the stabilize semaphore caps concurrent draining shards (still default 1) but the per-shard wait is "as long as actually needed."

Implemented on `feat/faster-mass-reconnect-phase1`. New env `IDENTIFY_STABILIZE_USE_DRAIN` (default true) enables drain-based wait; set to false to fall back to Phase 1 fixed-sleep semantics. `IDENTIFY_STABILIZE_SECONDS` becomes the floor (default 5s with drain on, 60s with drain off); `IDENTIFY_STABILIZE_SECONDS_MAX` is the cap (default 120s, decoupled from the floor). Batcher-queue-depth signal deferred — chunk-drain alone is sufficient for first cut.

**Phase 3a — Divergence-only check at GUILD_CREATE.**

Replace the unconditional `requestGuildMembers` on GUILD_CREATE with a divergence check: request only when `cached_count == 0` (cold) or `|discord_count - cached_count| / discord_count > BACKFILL_DIVERGENCE_RATIO` (default 0.05). Adds one knob and one in-process comparison; no schema changes. Steady-state mass identify on an already-populated DB skips most member requests entirely — reconnect approaches the Discord-floor lower bound.

**Phase 3b — Bounded-rate cursor sweep.**

Implement the manager-level goroutine described above. Knobs: `BACKFILL_REQUESTS_PER_SECOND` (default 12, sized for ~24h full sweep over 1M guilds), `BACKFILL_SWEEP_BATCH` (default 200), `BACKFILL_SWEEP_BATCH_INTERVAL` (default `BATCH / REQUESTS_PER_SECOND`). Persist the cursor as a single row. Sweep bounds drift independently of the connect path.

Each phase is independently shippable. Phase 1 is the structural change; Phases 2 and 3 progressively reduce the stabilize wait toward zero in steady state.

## Expected impact

| Phase | Mass identify of 1024 shards | Notes |
|---|---|---|
| Today | ~75 min | 64 × 70s, lock dominates |
| Phase 1, default config | ~64 min | Stabilize runs in parallel with subsequent identifies, but with `IDENTIFY_STABILIZE_CONCURRENCY=1` the stabilize gate becomes the new serial bottleneck (64 × 60s). Phase 1 alone is mostly about unlocking the tuning surface, not shipping a smaller wall. |
| Phase 1 + concurrency raised | ~8–16 min | Once metrics confirm the batcher absorbs N concurrent draining shards, raise `IDENTIFY_STABILIZE_CONCURRENCY`. With N=4 → ~16 min; N=8 → ~8 min. |
| Phase 2 | ~6–10 min typical | Drain signal beats fixed 60s for most guilds; concurrency knob still applies |
| Phase 3a + 3b (steady state) | ~5–8 min (Discord floor) | Most guilds skip backfill entirely; sweep handles staleness out-of-band |

## Open questions

None blocking. Items deferred to implementation:

- Exact metric names + label cardinality.
- Whether the cached `member_count` for the divergence check is computed on demand from member rows (cheap if a counter index exists) or maintained as a column on guilds via the batcher. Decision based on measured cost in Phase 3a implementation.
- Where the sweep cursor row lives — extending the existing `shards` table vs a new tiny `sweep_state` table.
- Whether to expose stabilize-semaphore tuning via the existing gRPC management API for live ops.
