# Progress Log — gateway

> Per-repo progress log. For cross-repo work, see `../../agent-progress.md` at the workspace root.

## Current Verified State

- Repository root: this directory, the `gateway` submodule under the agent-workspace's `repos/`
- Standard startup path: `./init.sh`
- Standard verification path: `go build ./...`
- Standard start command: `(cd cmd/gateway && go run .)`
- Current highest-priority unfinished feature: harness bootstrap — first `./init.sh` run not yet attempted
- Current blocker: none

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

### Session 002 — 2026-06-19

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
- Commits: on branch `chore/remove-fdb-backend`, not yet committed/pushed (awaiting maintainer authorization).
- Files or artifacts updated: cmd/gateway/main.go, cmd/state/main.go, internal/state/api/api.go, internal/state/db/statefdb/* (deleted), go.mod, go.sum, Dockerfile.gateway, Dockerfile.state, init.sh, README.md, AGENTS.md; workspace docs/repos/gateway.md, docs/stack.md.
- Known risk or unresolved issue: This is a behavior change — Postgres is now mandatory; any deployment relying on the implicit FDB default must set the psql address. Deployment manifests (helm/local-stack) were not audited in this session.
- Next best step: audit deploy configs (helm, local-stack) for any gateway/state env that assumed FDB or omitted the psql address, then commit and open a PR.
