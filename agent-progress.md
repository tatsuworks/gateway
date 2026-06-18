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

### Session 002c — 2026-06-18 (surgical force-identify control)

- Goal: Add an in-process per-shard force-identify path so staging/prod can exercise the READY/backfill-marker RGM-skip path without deleting shard rows and racing the old pod's graceful shutdown persist, and without restarting all 64 shards in a pod.
- Finding: `force-reload-guild.sh` and `reidentify.sh` delete shard rows before a graceful pod delete; the running process can reinsert the old resume tuple via `persistShardInfo` before the replacement reads it. A SIGKILL would avoid that race but would also skip safe final seq persistence for the other shards in the pod, making their resume rows stale.
- Completed:
  1. Added `force_identify` to `gatewaypb.RestartShardRequest` and regenerated `gatewaypb/gateway.pb.go`.
  2. Added `(*gatewayws.Session).ForceIdentify()`, which clears `seq`/`sessID`/`resumeURL`, persists the cleared tuple, then cancels only that shard's active context.
  3. Updated `Manager.RestartShard` to call `ForceIdentify` when `force_identify` is true; existing restart behavior remains a plain `Cancel`.
  4. Made `shouldResume` and `persistShardInfo` read `seq` with `atomic.LoadInt64`, matching the existing atomic writes in the read loop.
  5. Added tests for `ForceIdentify` clearing/persisting/canceling and protobuf round-trip of `force_identify`.
- Verification run:
  - `./init.sh` → success (dependency sync + baseline verification).
  - `GOCACHE=/tmp/go-build-cache go test ./internal/gatewayws ./gatewaypb ./internal/manager` → PASS (`internal/manager` has no test files).
  - `GOCACHE=/tmp/go-build-cache go build ./...` → success.
  - `GOCACHE=/tmp/go-build-cache go test ./discord/... ./internal/gatewayws/... ./gatewaypb` → PASS.
  - `GOCACHE=/tmp/go-build-cache go vet ./...` → clean.
- Notes: `gen.sh` still contains the old `GO111MODULE=off go get` generator install command, which fails on this Go toolchain. The generated protobuf was refreshed with `PATH="$(go env GOPATH)/bin:$PATH" protoc -Igatewaypb --gogofaster_out=plugins=grpc:gatewaypb gatewaypb/gateway.proto` after installing `protoc-gen-gogofaster@v1.3.2`.
- Next best step: deploy the branch to staging, call `RestartShard` with `force_identify: true` for shard 0, and confirm logs show `force identifying shard` → `sending identify` → `ready` → `preload guild backfill times`/RGM skip behavior.

### Session 002d — 2026-06-18 (force-identify thread-safety fix + staging validation)

- Goal: Fix a data race found while reviewing Session 002c's force-identify, then run the staging end-to-end validation that was the last gate on the feature.
- Findings (review of 002c's `ForceIdentify`):
  1. `ForceIdentify` mutated the read-loop-owned `seq`/`sessID`/`resumeURL` from the gRPC handler goroutine while the read loop was live — a data race (torn string reads on `sessID`/`resumeURL`). The 002c `seq` atomic conversion was also incomplete (plain writes remained on the invalid-session and 4006 paths, plain read in the resume payload).
  2. Persist/cancel ordering window: the cancelled connection's deferred `persistShardInfo` could rewrite the stale resume tuple after the eager clear.
- Completed (commit `58195f2`):
  1. `ForceIdentify` is now a pure signal — sets an atomic `forceIdentify` flag and cancels; no field mutation on the caller goroutine.
  2. The clear moved to `applyForceIdentify`, run on the read-loop goroutine at the top of `Open`, so `sessID`/`resumeURL` stay single-goroutine.
  3. `persistShardInfo` is flag-aware: while a force is pending it persists a cleared tuple, closing the deferred-persist window.
  4. Completed the `seq` atomic conversion (invalid-session, 4006, resume payload, ready-resume log).
  5. Fixed `gen.sh` (`go install ...@v1.3.2` + `PATH`; the removed `GO111MODULE=off go get` form is gone) — regenerates `gatewaypb` with zero diff.
  6. Tests rewritten to the signal/apply split + a `-race` concurrency test (1000 `ForceIdentify` calls vs a stand-in read loop).
- Verification (local): `go build ./...`, `go vet ./...`, `go test -race ./internal/gatewayws/... ./gatewaypb`, `./gen.sh` (zero diff) — all pass.
- Staging validation (deployed `gateway:58195f2` via `helm`; staging runs `shards: 1`, so all guilds are on shard 0):
  - Force-identify shard 0 → logs `force identifying shard` → `force identify requested, discarding resume state` → `sending identify` (NOT `sending resume`) → `ready` with a fresh session id. Thread-safe signal/apply path confirmed end-to-end.
  - Small guild (≤250): delete members + marker, force-identify → members repopulate from the inline GUILD_CREATE roster with no RGM; `guild_backfills` re-stamped by the reconcile path. Confirms small-guild reconcile + no-RGM-storm.
  - Large guild (>250, ~1.2k members): delete marker, force-identify → `members` refills to ~1.2k. A >250 guild's GUILD_CREATE cannot carry the full offline roster inline (large_threshold=250), so the only source is `GUILD_MEMBERS_CHUNK` — i.e. the automatic RGM fired and the chunk path stamped the marker. 1.2k members = 2 chunks, so **multi-chunk** assembly (chunk_index 0→1, `Complete` on the last) was exercised too.
  - Skip gate: with a marker present, RGM/reconcile are bypassed.
- Key finding (observability, not a bug): the AUTOMATIC RGM path (`requestGuildMembers`, `internal/gatewayws/write.go:187`) logs nothing on success; only the MANUAL gRPC path (exported `RequestGuildMembers`) logs `"sending members request"`. So that grep is NOT a valid auto-RGM signal — verify via the DB (member count / marker) instead. Left unlogged deliberately (prod log volume).
- gRPC testing note: gogofaster + grpc-reflection makes grpcurl *invoke* fail with "does not expose service" even though `list` works; pass `-proto gatewaypb/gateway.proto` to bypass reflection.
- Next best step: open the PR to `master`; after merge + deploy, seed the prod baseline via one full re-IDENTIFY.
