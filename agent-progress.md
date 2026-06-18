# Progress Log — gateway

> Per-repo progress log. For cross-repo work, see `../../agent-progress.md` at the workspace root.

## Current Verified State

- Repository root: this directory, the `gateway` submodule under the agent-workspace's `repos/`
- Standard startup path: `./init.sh`
- Standard verification path: `go build ./...`
- Standard start command: `(cd cmd/gateway && go run .)`
- Standard test path (no infra): `go test ./discord/... ./internal/gatewayws/...`
- Standard DB integration test: `go test ./internal/state/db/statepsql/...` (needs a Postgres with the `state` schema; repo convention DSN is `tatsu@localhost`)
- Current highest-priority unfinished feature: backfill-marker RGM skip — implemented and verified locally (build/vet/unit + statepsql integration against live PG14); pending push, PR, and staging validation
- Current blocker: none (staging requires `helm`-based deploy + applying `guild_backfills` to the staging `state` DB)

## Session Log

### Session 001 — 2026-05-25

- Goal: Harness scaffold added to repo as part of workspace bootstrap.
- Completed: Wrote `AGENTS.md` (merged or fresh as appropriate), `init.sh`, this `agent-progress.md`, and `feature_list.json`. `CLAUDE.md` reduced to a one-line pointer to AGENTS.md.
- Verification run: none — `./init.sh` not yet executed.
- Evidence captured: file presence only.
- Commits: none yet — harness files uncommitted, pending review before per-repo commit.
- Files or artifacts updated: AGENTS.md, CLAUDE.md (pointer), init.sh, agent-progress.md, feature_list.json
- Known risk or unresolved issue: `init.sh` commands are inferred from the existing repo conventions. Gateway needs FoundationDB client libs (v6.2.27). Runtime requires Redis + etcd + Discord TOKEN. The verify command runs a build only — full tests require infra.
- Next best step: run `./init.sh` from this directory. Fix any command that does not work, update this log, then commit the harness files.

### Session 002 — 2026-06-18

- Goal: Implement the backfill-completion-marker RGM-skip design (`docs/superpowers/specs/2026-06-12-backfill-marker-rgm-skip-design.md`) per the plan at `docs/superpowers/plans/2026-06-18-backfill-marker-rgm-skip.md`.
- Completed (all 6 plan tasks):
  1. `discord.MemberChunk` gains `ChunkIndex`/`ChunkCount`; both ETF and JSON decoders extract them. Fixed a latent ETF desync bug (the `default:` case in `DecodeMemberChunk` left unknown keys unconsumed) by skipping with `readTerm()`.
  2. `guild_backfills` UNLOGGED marker table added to `init.sql`/`down.sql`.
  3. Four `state.DB` methods (`BeginGuildBackfill`, `CompleteGuildBackfill`, `ReconcileGuildMembers`, `GetGuildBackfillTimes`): real in `statepsql`, no-op in `statefdb` (PSQL-only optimization).
  4. Handler stamps begin/complete markers in `MemberChunk`; `GuildCreate` now returns an enriched `*EventPayload` (`MemberCount`/`ReceivedMembers`/`MemberIDs`).
  5. Session preloads markers in bulk on READY and gates RGM via a skip switch (fresh marker → skip; small full guild → reconcile from payload; else RGM), with `BACKFILL_STALENESS_HOURS` (default 24h) and per-guild expiry jitter [0.75,1.25).
- Verification run (commands + results):
  - `go build ./...` → success.
  - `go vet ./...` → clean.
  - `go test ./discord/... ./internal/gatewayws/...` → PASS (decoder field tests, `membersComplete`, `jitterFactor` bounds/determinism).
  - `go test ./internal/state/db/statepsql/ -run 'Backfill|Reconcile|Complete'` → all 4 PASS against live PostgreSQL 14 (local-stack container). The committed test uses the repo-convention DSN `tatsu@localhost`; in this environment it was run against `postgres:password@localhost/state` (the only auth this container accepts) after creating `guild_backfills` there.
- Evidence captured: command output above; the integration run surfaced a test-seeding bug (app-clock vs DB-clock skew) which was fixed — see commit `test(statepsql): seed last_updated from the DB clock`.
- Commits: 7 on `feat/backfill-marker-rgm-skip` (plan doc + 5 feature commits + 1 test fix). Not pushed (awaiting maintainer authorization).
- Files or artifacts updated: `discord/types.go`, `discord/discordetf/member.go`(+test), `discord/discordjson/member.go`(+test), `internal/state/state.go`, `internal/state/db/statepsql/{init.sql,down.sql,backfills.go,backfills_test.go}`, `internal/state/db/statefdb/backfills.go`, `handler/{helpers.go,guild.go,member.go}`, `internal/gatewayws/{backfill.go,backfill_test.go,ws.go,write.go}`.
- Known risk or unresolved issue: Not yet exercised end-to-end against a live gateway/Discord connection (decode of real `chunk_index`/`chunk_count`, preload at scale, reconnect-storm read volume). Staging validation pending — gateway is deployed via the `helm` repo (`yarn switchStaging` → `staging_deploy.sh`). The `guild_backfills` table must be applied to the staging `state` DB before deploy (`init.sql` is idempotent / `IF NOT EXISTS`).
- Next best step: push the branch (with authorization), open the PR, then build the gateway image and deploy to staging via `helm`; watch RGM-skip rate and PSQL read volume on a small shard range.

### Session 002b — 2026-06-18 (design pivot: no-expiry skip)

- Goal: Make the marker actually help the priority case — a mass re-IDENTIFY after long uptime. With the original jittered ~18–30h skip expiry, every marker is stale past 30h, so the expensive storm re-backfills all guilds (no relief). The superseded `GuildHasMembers` EXISTS branch skipped that storm precisely because it never expired — but at the cost of permanently partial/drifting rosters.
- Decision: skip is now **no-expiry** — skip RGM for any guild with `backfilled_at IS NOT NULL`, regardless of age. The completion marker is only stamped on the final chunk / small-guild reconcile, so this never skips a partial roster (dominates EXISTS on the speed path). Roster drift is bounded out-of-band: the reconciliation delete on each real backfill + the (now load-bearing) Phase 3b background sweep. `BACKFILL_STALENESS_HOURS` + jitter move from "skip expiry" to "sweep cadence."
- Completed: dropped the `isFresh` staleness filter from the READY preload (`internal/gatewayws/ws.go`); retargeted `backfillStalenessWindow`/`isFresh`/`jitterFactor` doc comments to the sweep (retained for Phase 3b, currently unused by the skip path). Added a revision banner to the design doc and a revision note to the plan.
- Verification run: `go build ./...` success; `go vet ./internal/gatewayws/` clean; `go test ./internal/gatewayws/` PASS.
- Decision (drift): Phase 3b sweep is **deferred / optional**, not required. Drift only accrues during disconnect windows (live member events keep rosters accurate while connected), so good uptime keeps it small; the UNLOGGED cache is also reset wholesale on any unclean PG restart, forcing cold re-backfills. Build the sweep only if roster staleness becomes a real complaint. Watch items: drift peaks during a rare long fleet-wide outage (skip declines to reconcile then), and role/permission queries over a ghost return a stale positive (low impact — ghosts are dormant rows).
- Next best step: **staging validation** of the branch as-is — confirm real Discord `chunk_index`/`chunk_count` stamp `guild_backfills` markers, the no-expiry skip fires on reconnect, and small-guild payload reconcile works. Requires: push (maintainer authorization) → build gateway image → apply `guild_backfills` (init.sql, IF NOT EXISTS) to the staging `state` DB → deploy via `helm` (`yarn switchStaging` → `staging_deploy.sh`/`restartGateway`) on a small shard range. Then seed the prod baseline via one full re-IDENTIFY.
