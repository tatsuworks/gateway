# Progress Log — gateway

> Per-repo progress log. For cross-repo work, see `../../agent-progress.md` at the workspace root.

## Current Verified State

- Repository root: this directory, the `gateway` submodule under the agent-workspace's `repos/`
- Standard startup path: `./init.sh`
- Standard verification path: `go build ./...`
- Standard start command: `(cd cmd/gateway && go run .)`
- Standard test path (no infra): `go test ./discord/... ./internal/gatewayws/...`
- Standard DB integration test: `go test ./internal/state/db/statepsql/...` (needs a Postgres with the `state` schema; repo convention DSN is `tatsu@localhost`)
- Current highest-priority unfinished feature: TATSU-2514 — stale `resume_gateway_url` 503 redial loop; implemented and verified locally (build/vet/`go test ./...`/`-race`), pending staging validation and a prod deploy
- Current blocker: none (staging validation requires a `helm`-based deploy)

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

### Session 003 — 2026-06-19

- Goal: Remove the deprecated FoundationDB (FDB) state backend, leaving Postgres as the sole store.
- Completed:
  - Deleted `internal/state/db/statefdb/` (entire FDB implementation, 11 files).
  - `cmd/gateway/main.go` & `cmd/state/main.go`: dropped the statefdb import and the FDB fallback branch; Postgres address is now required (binaries `Fatal` if `-psqlAddr`/`-psql` is empty).
  - `internal/state/api/api.go`: removed the now-dead `apple/foundationdb` import and unused `FDBRangeWantAll` var (statefdb had its own copy in `helpers.go`).
  - `go mod tidy` dropped `github.com/apple/foundationdb/bindings/go` from go.mod/go.sum.
  - Dockerfile.gateway & Dockerfile.state: removed the FDB client-lib `.deb` install steps (both build and runtime stages).
  - Docs: init.sh comment, README.md, AGENTS.md (Repo Context), workspace `docs/repos/gateway.md` and `docs/stack.md` updated to reflect Postgres-only storage.
- Verification run: `go build ./...` (exit 0), `go vet ./...` (exit 0), `go test ./discord/... ./internal/state/...` — all pass except `statepsql.TestChannels`, which fails only because it needs a live Postgres (pre-existing infra dependency, password auth), unrelated to this change.
- Evidence captured: build/vet/test output in session; foundationdb absent from go.mod/go.sum confirmed via grep.
- Note (2026-06-22 merge): merged after #75 (backfill-marker RGM skip); the no-op `statefdb/backfills.go` that #75 added to satisfy the interface was removed here with the rest of the FDB backend.
- Known risk or unresolved issue: This is a behavior change — Postgres is now mandatory; any deployment relying on the implicit FDB default must set the psql address. helm #59 drops the dead FDB mount from the gateway/state templates (merged alongside this).
- Next best step: deploy to staging via `helm` and confirm gateway/state boot Postgres-only.

### Session 004 — staging + PROD deploy of #75 + #76 (2026-06-22)

- Goal: ship the merged force-identify + backfill-marker RGM-skip (#75) and FDB removal (#76) to staging then production via `helm`.
- Merges: #75 (`627a790`) and #76 (`7118fe0`) to `master`; #76 needed a conflict resolution — #75 had added a no-op `internal/state/db/statefdb/backfills.go` to satisfy the new backfill interface for the FDB backend, removed here with the rest of `statefdb`. Post-merge `go build`/`vet`/tests clean; `foundationdb` absent from go.mod/go.sum.
- **Two Docker regressions in #76, found at image-build/deploy time and fixed before prod** (#76 deleted an `apt install` line that bundled non-FDB deps):
  - **#77** (`92b1bcd`): restored `zlib1g/zlib1g-dev` — required by `github.com/tatsuworks/czlib` (CGO zlib-stream decoder in `internal/gatewayws`), independent of FDB. Without it the Docker *build* failed: `pkg-config … Package zlib was not found`.
  - **#78** (`67d0133`): restored `ca-certificates` — was pulled in transitively by the removed `wget`; `ubuntu:24.04` ships no CA bundle, so the gateway *runtime* hit `tls: x509: certificate signed by unknown authority` dialing Discord and reconnect-looped (pod stayed Running, 0 connections). Added to the runtime stage of both Dockerfiles.
- Image deployed: `gateway`/`state` `67d0133-release` (includes #75 + #76 + #77 + #78).
- **Staging** (helm rev 707): gateway-0 + state 1/1 Running, 0 restarts, Postgres-only boot; identify→ready (34 guilds), resuming cleanly; no x509.
- **Production** (helm rev 2327, deployed by maintainer): gateway StatefulSet 16/16 + state 16/16 ready on `67d0133-release`. **All 1024 shards RESUMED** on the rolling restart — fleet-wide log scan: 0 `sending identify`, 0 `x509`, 0 fatal/panic across every pod; ~64 `resumed`/pod. The graceful-persist→load-on-startup→`shouldResume` path carried the whole fleet; **no RGM storm** (resume replays the stream rather than re-backfilling).
- Verification (prod): `helm history` rev 2327 `deployed`; pod images + `kubectl get statefulset/deploy` 16/16 ready; per-pod `kubectl logs` grep for x509/fatal/identify = 0. `guild_backfills` confirmed present in prod `state` DB; READY-preload (`ws.go:470`) + marker stamping (`handler/member.go:22-30`, `backfill.go:70`) all log-and-continue on a table error → no fleet-outage path.
- Known risk / next step: because every shard resumed, **no backfill markers were stamped and the RGM-skip path never fired** — the feature is live but unexercised in prod. To seed the baseline (so a future mass re-identify skips RGM), run one controlled full re-IDENTIFY into the now-present `guild_backfills` table. Rollback: re-pin gateway/state → `9c68d57`/`1f814af-release` (or `helm rollback tatsu 2326`).

### Session 005 — remove local write-hook additions (2026-06-23)

- Goal: Remove the two local-only write-path hook additions from `fix/ws-send-deadlock-on-writer-exit` while preserving the upstream heartbeat/dead-writer fix.
- Completed: removed the nonblocking `requeue` helper/path and its test; restored `writeOp` to defer `w.Close()` without separately surfacing flush errors; restored the upstream `heartbeatStale(now time.Time)` test shape. `internal/gatewayws/write.go` and `internal/gatewayws/heartbeat_deadlock_test.go` now have no diff versus `origin/fix/ws-send-deadlock-on-writer-exit`.
- Verification run:
  - `./init.sh` -> exit 0; ran dependency sync and baseline `go build ./...` (Go emitted a non-fatal read-only module stat-cache warning in this sandbox).
  - `go test ./discord/... ./internal/gatewayws/...` -> PASS before edits.
  - `env GOCACHE=/tmp/go-build-cache go test ./internal/gatewayws` -> PASS.
  - `env GOCACHE=/tmp/go-build-cache go test ./discord/... ./internal/gatewayws/...` -> PASS.
  - `env GOCACHE=/tmp/go-build-cache go build ./...` -> exit 0; same non-fatal read-only module stat-cache warning.
- Known risk or unresolved issue: none for this cleanup. The branch remains locally ahead/behind its upstream because the upstream branch has the same subject at a different commit SHA; no push was performed.

### Session 006 — conn-lifecycle refactor (Session → Session + per-Open conn) (2026-06-23)

- Goal: retire the bug class behind the WS-deadlock fixes by restructuring, rather than patching, `internal/gatewayws`. Split the reused-across-reconnects `Session` god-object into a durable `Session` (identity, shared deps, resume tuple) and a fresh-per-`Open()` `conn` (ctx/cancel, wch/prioch, websocket, buffers, heartbeat timing). Branch `refactor/conn-lifecycle` off `fix/ws-send-deadlock-on-writer-exit` HEAD (`778d772`, which includes the bounded-write + writeOp flush-error check).
- Plan: `docs/superpowers/plans/2026-06-23-conn-lifecycle-refactor.md` (written, codex-reviewed; all 8 must-fix items folded in before execution).
- Completed (commits `93e1585`, `d0a090d`, `1bea7f8` + plan `89731fb`):
  - New `conn` type owns per-connection state; `Session` keeps a `cur atomic.Pointer[conn]` that management RPCs (`Cancel`/`ForceIdentify`/`RequestGuildMembers`/`Status`/`LongLastAck`) route through.
  - Deleted the three reconnect-reset hacks a fresh conn makes unnecessary: `resetHeartbeat`, the identify-path `wch`/`prioch` swap, and the INVALID_SESSION `s.wch = make(...)` rebuild.
  - Preserved behavior deliberately: `guilds`/`backfilled` stay on `Session` (read-loop single-owner, survive RESUME so the backfill-skip optimization holds); `last` atomic on `Session`; `run(parent)` keeps the two-context split so `state.HandleEvent`/`maybeRequestGuildMembers` still run under the parent ctx (in-flight DB work survives a disconnect); `Open` cancels the connection before clearing `cur`.
  - Closed the pre-existing `lastAck`/`curState` data race: `lastHB`/`lastAck`/`ready`/`curState` are now behind a `conn.mu` (setState/markHB/markAck/markReady/snapshot); `authed` is `atomic.Bool`.
  - Two blessed, documented behavior changes: `RequestGuildMembers` is dropped+logged when no connection is active; `Status`/`LongLastAck` report `<disconnected>`/`true` on nil `cur`.
- Verification: `go build ./...` clean; `go vet ./...` clean; `go test -race ./internal/gatewayws/ ./internal/manager/ ./gatewaypb/` PASS (19 gatewayws tests incl. new channel-independence, nil-cur, and Status-race tests). External API signatures (`go doc Session`) unchanged. Greps confirm no reset-hacks and no stale `s.<conn-field>` access remain.
- Known risk / next step: not deployed (per AGENTS.md, shipping is a separate helm staging-deploy). Branch not pushed (awaiting maintainer go-ahead). NOTE: the agent-progress Session 005 entry describes the deadlock branch with `defer w.Close()` and no flush-error surfacing, but this refactor was based on `778d772` which *does* surface the writeOp flush error — the two narratives diverge; reconcile when the deadlock PR and this refactor are sequenced for merge.

### Session 007 — staging validation + code review + fixes for PR #81 (2026-06-23)

- Goal: assess prod-readiness of PR #81 (WS send-deadlock fix + conn-lifecycle refactor; the only delta over the already-shipped `origin/master`), run the missing staging IDENTIFY validation, then review and fix.
- **Staging validation** (deployed `gateway:2b2ecb7-release` = branch HEAD; `SHARDS=1`, all guilds on shard 0):
  - Baseline: shard reconnect-loops every ~31s — `sending resume` → `resumed`, then a 30s `readMessage` deadline (`connectionTimeout=30`, read.go:13) trips between sparse events on the quiet 34-guild staging shard → `took too long to get reader` → reconnect. Confirmed **expected/known** for low-traffic staging (prod shards are busy, never trip it). Side benefit: the resume/persist path is exercised ~113×/hr, every cycle succeeding — the refactored RESUME path is well-proven.
  - **Force-identify** (gRPC `RestartShard{shard:0, force_identify:true}` via port-forward + `-proto`): `force identifying shard` → `force identify requested, discarding resume state` (applyForceIdentify on the read-loop goroutine) → `sending identify` (not resume) → `ready` with fresh session `214fcd…`. IDENTIFY path confirmed.
  - **Post-identify**: next cycles RESUMED the *new* session, 0 stray identifies → IDENTIFY→persist→`shouldResume`→RESUME loop confirmed on the refactored code.
  - Health: 0 restarts, 61 MiB after ~280 reconnects (no conn/goroutine leak), 0 panics.
- **Code review** (8 finder angles + read-the-code verification on `origin/master...HEAD`): deadlock genuinely fixed (writer `defer cancel()` + ctx-guarded sends + bounded `writeOp` w/ flush-error check); removed-behavior audit clean (all invariants re-established by the per-conn lifecycle). Findings were secondary.
- **Fixes applied (this session, commit below):**
  1. **Identify-lock release regression (the one observed live as `release identify lock after ready: context canceled`)**: `releaseIdentifyLock` (ws.go) unlocked on the per-conn `c.ctx`; the refactor regressed this from origin/master's durable `s.ctx`. The post-READY release waits `calcIdentifyWait` (up to ~70s) and the INVALID_SESSION release runs mid-teardown, so `c.ctx` is cancelled by then → unlock failed → the cross-shard identify mutex leaked to lease TTL (worst during the reconnect storms this PR targets). Now unlocks on a `context.Background()`+10s ctx (etcd client is Session-durable). Fixes both release sites (post-READY ws.go:500, INVALID_SESSION ws.go:436).
  2. **Comment accuracy**: `requestGuildMembers` (write.go) no longer claims recovery "via the staleness sweep" (deferred/unbuilt) — states next full IDENTIFY or manual RGM.
  3. **`rotateStatuses` landmine**: bare `wch` send → ctx-guarded `select` so re-enabling the (commented-out) caller can't reintroduce the writer-exit wedge.
- **Deliberately deferred** (rationale): #2 etcd-session prompt-revoke (with #1 fixed, only an idle lease lingers ~80s to TTL; no goroutine leak; a prompt-revoke touches the rate-limiter and Session.Close hits the same `c.ctx` cancellation) — accepted as-is; `persistence.go` `context.Background()` DB calls (bounded ctx is a real behavior change, needs deliberate sizing); dead `backfill.go` Phase-3b scaffolding (intentionally retained).
- Verification: `go build ./...`, `go vet ./internal/gatewayws/...`, `go test -race ./internal/gatewayws/...` all clean post-fix.
- Next best step: rebuild the gateway image, redeploy to staging, force-identify shard 0, and confirm the `release identify lock after ready: context canceled` log is GONE (the conn reconnects at 30s while the ~70s release goroutine later unlocks on a live context). Then get a review pass on PR #81 and merge.

### Session 008 — TATSU-2514: stale `resume_gateway_url` traps a shard in a permanent 503 redial loop (2026-09-01)

- Goal: fix the production incident in [TATSU-2514](https://linear.app/tatsu-works/issue/TATSU-2514) — 41 of 1024 prod shards stuck redialing a dead Discord edge, fully unresponsive, with zero self-healing and no alerting (pods stayed `1/1 Running`).
- Root cause (confirmed by reading the code, matching the ticket): `GatewayURL()` preferred `resumeURL` unconditionally whenever the tuple was non-empty. When the edge behind that URL stops serving the shard, the failure lands on the **WebSocket handshake** (`503`, before IDENTIFY/RESUME is sent), so Discord never gets to invalidate the session via `INVALID_SESSION` — the signal that would normally demote the resume tuple never arrives. Nothing else in the connect path touched `resumeURL`, and `loadResumeURL` reloads it from the `shards` table on start, so a pod restart walks straight back into the same loop.
- Implemented (TDD — tests written first and watched fail):
  1. **`internal/gatewayws/resume.go` (new)**: `MaxResumeDialFailures = 3`, `usingResumeURL`, `noteDialSuccess`, `noteDialFailure`. Three consecutive dial-stage failures against a resume host clear `seq`/`sessID`/`resumeURL` in memory and persist the cleared tuple, so both the next dial and any later pod restart use `wss://gateway.discord.gg/`. This is the automatic trigger for the mechanism `ForceIdentify` already proved in production; the manual `yarn gwForceIdentify` workaround recovered a shard in 4.7s.
  2. **`internal/gatewayws/ws.go`**: the `websocket.Dial` site captures `usingResumeURL()` before dialing and reports the outcome. Failures on a **cancelled** connection context (shutdown / `Cancel` / `ForceIdentify`) are our own teardown and are not counted. `GatewayURL` now branches on `usingResumeURL` so the dial-site classification and the URL decision cannot drift apart. New `Session.resumeDialFailures` field is read-loop-goroutine-owned, same as `sessID`/`resumeURL`.
  3. **`internal/manager/reconnect.go` (new) + `manager.go`**: the flat `time.Sleep(time.Second)` retry becomes an exponential, jittered, ctx-cancellable backoff (`reconnectDelay` 1s→60s, `nextFailureCount`, `sleepCtx`). A connection that lasts ≥60s resets the ladder. The flat retry generated ~3,300 error lines in 10 minutes on gateway-14 alone, which rotated the log buffer fast enough to destroy the evidence of when the loop started. The `websocket closed` line now carries `uptime` / `consecutive_failures` / `retry_in`.
- Threshold rationale: recovery costs a full re-IDENTIFY and shard re-backfill (~1,400 guilds, historically ~40min on prod), so a single transient dial blip must not trigger it — hence 3 consecutive, reset by any successful dial.
- Verification (all run this session): `./init.sh` exit 0; `go build ./...` exit 0; `go vet ./internal/...` clean; `go test ./...` all packages ok; `go test -race -count=2 ./internal/gatewayws/... ./internal/manager/...` PASS. 12 new tests (6 gatewayws, 6 manager), each watched failing first.
- Test-harness note: `newResumableSession` (force_identify_test.go) now sets `enc: stubEncoding{}` — `GatewayURL()` dereferences `enc.Name()`, which the helper previously left nil.
- Known risk / not done:
  - **No staging validation yet** (needs a `helm` deploy). Repro to run there: write a bogus but resolvable `resume_gateway_url` into the shard's `shards` row, restart the pod, and confirm `resume gateway url unreachable, discarding resume state` → `sending identify` → `ready` rather than a 503 loop.
  - **Ticket bullet 4 deliberately out of scope** (it is phrased as "Consider"): a metric/alert on "shard has not reached `ready` in N minutes". This class of outage remains invisible to monitoring; `logHealth` does list disconnected shards in its 5-minute `shard report`, but nothing pages.
  - The 41 shards stuck at time of filing are not recovered by merging this — they need the existing `gwForceIdentify` workaround (batched, per `reidentify`'s settle gating) or a deploy carrying this fix.
- **Codex review follow-up (`codex review --base master`), 1 × P2, confirmed and fixed:** the new backoff regressed management restarts. `Open` clears `Session.cur` on return, so for the whole reconnect wait `Session.Cancel` is a no-op while `RestartShard` still returns success — the dead window grew from ~1s (flat retry) to up to ~75s, landing squarely on `gwForceIdentify`, the operator workaround for this very incident (ticket evidence: recovery in 4.7s). Codex also noted that cancelling a short-lived connection escalates the failure ladder. Fixed with a per-shard depth-1 wake channel owned by the `Manager` (`wake map[int]chan struct{}` + `wakeShard`, guarded by `shardMu`): `RestartShard` signals it after `Cancel`/`ForceIdentify`, `sleepCtx` became `waitBeforeReconnect(ctx, wake, d) (ok, woken bool)`, and a woken wait resets `failures` to 0 so an operator-requested reconnect gets the prompt first attempt. Non-blocking (never stalls the gRPC handler), coalescing, and a signal sent while the loop is not waiting is held for its next wait. 5 new/rewritten manager tests, watched failing first. `ForceIdentify`'s atomic flag always survived this window regardless — it is consumed by the next `Open` — so the bug was latency, not a lost force-identify.
- **Staging validation RAN and PASSED (2026-09-01, image `gateway:3a2e5ae`, helm revision 959).** Repro: scaled the gateway StatefulSet to 0 (required — `conn.run`'s `defer persistShardInfo()` means a running shard writes its real tuple back on shutdown and would overwrite the injection), set `shards.resume_url = 'wss://example.com'` for shard 0 leaving `sess`/`seq` intact, scaled back to 1. Observed:
  - `08:03:36.364` attempt 1 → `dial gateway: ... expected handshake response status code 101 but got 200`, `consecutive_failures=1`, `retry_in=2.072s`
  - `08:03:38.551` attempt 2 → same, `consecutive_failures=2`, `retry_in=4.276s`
  - `08:03:42.846` attempt 3 → **`resume gateway url unreachable, discarding resume state`** (`resume_gateway_url=wss://example.com`, `consecutive_dial_failures=3`)
  - `08:03:51.851` attempt 4 → **`sending identify`** (not `sending resume`) → `08:03:52.195` `ready`, fresh `sess=7e4dd6c957b37078ea58f3124dc8633b`, `resume_gateway_url=wss://gateway-us-east1-c.discord.gg`, `guild_count=39`
  - `gateway-0` 1/1 Running, **0 restarts**; the `shards` row afterwards holds the new session and a real discord.gg edge. Total escape time **15.8s**, versus never without the fix.
- **Two findings from the validation run, neither blocking:**
  1. **The post-invalidation backoff wait is dead time.** The ladder is not reset when the resume tuple is discarded, so attempt 4 waited `retry_in=9.003s` even though it was about to dial a *different* host (the main gateway URL) where the previous failures predict nothing. 9.0s of the 15.8s total was this wait. Resetting the failure count inside `noteDialFailure`'s invalidation branch would cut escape time to ~7s. Worth doing; needs a rebuild + redeploy + re-validate.
  2. **Duplicate `shard` field in the new log line** — `s.log` is already `.With(slog.F("shard", ...))` in `NewSession`, so `noteDialFailure`'s own `slog.F("shard", ...)` renders `{"shard":0,"shard":0}`. Cosmetic; `applyForceIdentify` has the same pre-existing duplication.
- Next best step: decide on finding 1 (reset the ladder on invalidation) — if taken, rebuild/redeploy/re-validate staging, then land the PR and deploy to prod. Note the 41 shards stuck at filing time still need `gwForceIdentify` or the prod deploy.
