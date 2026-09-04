# AGENTS.md — gateway

> This repo is a plain clone inside [`tatsuworks/agent-workspace`](https://github.com/tatsuworks/agent-workspace) at `repos/gateway` — listed in that workspace's `repos.txt` manifest, **not** a submodule. When checked out inside that workspace, workspace-wide rules live in [`../../AGENTS.md`](../../AGENTS.md). Repo-specific rules and context follow.

## Harness Operating Loop

Before writing code in this repo:

1. Run `pwd` and confirm you are inside the `gateway` clone (i.e. `<agent-workspace-root>/repos/gateway`).
2. Read your branch handoff at `handoffs/<branch-slug>.md` for the latest verified state and next step (slug = `git branch --show-current | tr '/' '-'`, falling back to `detached-$(git rev-parse --short HEAD)` when that is empty on a detached checkout; copy `handoffs/_template.md` if it doesn't exist yet).
3. Read the Linear ticket for the work you are picking up — it is the durable record of what is in flight, its done criteria, and its verification evidence.
4. Review recent commits: `git log --oneline -5`.
5. Run `./init.sh`.
6. Run the required smoke or end-to-end verification before starting new work.

If baseline verification is already failing, fix that first. Do not stack new feature work on top of a broken starting state.

## Harness Working Rules

- Work on one feature at a time.
- Do not mark a feature complete just because code was added.
- Keep changes within the selected feature scope unless a blocker forces a narrow supporting fix.
- Do not silently change verification rules during implementation.
- Prefer durable repo artifacts over chat summaries.
- Handoffs are working state, not history: `handoffs/*` is gitignored (only `_template.md` is committed), so nothing written there reaches a PR or `master`. Graduate anything durable before the branch ends — see Definition Of Done.
- This repo is a plain clone inside the workspace — tracked in its `repos.txt` manifest, **not** a submodule. Commit harness changes here and push them through this repo's own PR route; the workspace records no commit SHA, so there is no parent pointer to bump.

## Required Harness Artifacts

- `handoffs/<branch-slug>.md` — per-branch session log and current verified status; gitignored working state (copy from `handoffs/_template.md`)
- `init.sh` — standard startup and verification path

Durable status, done criteria, and verification evidence live on the **Linear ticket**, not in a committed file in this repo. See [the workspace decision log](../../docs/decision_log/retire-workspace-progress-trackers.md).

## Definition Of Done

A feature is done only when:

- The target behavior is implemented.
- The required verification actually ran.
- Verification evidence is recorded on the Linear ticket and in the PR body. A handoff is gitignored, so evidence left only there is lost with the branch.
- Anything durable has graduated out of the handoff: decisions → a decision log, system knowledge → `docs/`, process knowledge → this file.
- The repository remains restartable from `./init.sh`.

## End Of Session

1. Update your branch handoff (`handoffs/<branch-slug>.md`).
2. Update the Linear ticket with status and verification evidence.
3. Record any unresolved risk or blocker in the handoff.
4. Graduate anything durable out of the handoff — it is gitignored and dies with the branch.
5. Commit with a descriptive message once the work is in a safe state.
6. Leave the repo clean enough for the next session to run `./init.sh` immediately.

---

# Repo Context



## Project Overview

Discord Gateway & State Cache service for Tatsu. Two components:
- **Gateway** (`cmd/gateway/`) — Multiplexes many Discord WebSocket connections (720+ shards at peak, ~4 cores). Publishes events to Redis via RPUSH with ETF-encoded payloads.
- **State Cache** (`cmd/state/`) — HTTP/2 REST API for querying cached Discord state (guilds, channels, members, roles, messages, threads).

## Build & Run

```bash
# Gateway
cd cmd/gateway && go build -o gateway . && ./gateway

# State Cache
cd cmd/state && go build -o state . && ./state
```

## Testing

```bash
go test ./...                              # All tests
go test ./discord/discordjson/...          # JSON encoding tests
go test ./internal/gatewayws/...           # WebSocket session tests
go test -run TestFoo ./path/to/pkg         # Single test
go test -bench . ./discord/discordjson    # Benchmarks
```

## Code Generation

Protobuf generation for gRPC management API:
```bash
./gen.sh   # runs protoc with gogofaster plugin → gatewaypb/
```

## Architecture

### Event Flow
```
Discord WS → gatewayws.Session → handler.Client → state.DB (store) → Manager → Redis RPUSH → consumers (BLPOP)
```

### Key Packages
- `internal/gatewayws/` — WebSocket session lifecycle per shard (connect, read, write, heartbeat, identify)
- `internal/manager/` — Multi-shard orchestration, Redis event routing, gRPC management server
- `handler/` — Discord event processing (guild, channel, member, message, role, emoji, thread events)
- `internal/state/` — `state.DB` interface (~93 methods) abstracting the storage backend
- `internal/state/db/statepsql/` — PostgreSQL implementation (the only backend; the deprecated FoundationDB path has been removed)
- `internal/state/api/` — REST API handlers for state queries (fasthttp + httprouter)
- `discord/discordetf/` — Custom ETF (Erlang Term Format) encoder/decoder built from scratch
- `discord/discordjson/` — JSON encoding alternative
- `gatewaypb/` — Protobuf definitions for gRPC management API (Version, RequestGuildMembers, RestartShard, Stats)

### Coordination
- **etcd** — Distributed mutex for shard identify rate limiting
- **Redis** — Event queue with per-instance event type filtering via `MULTI_REDIS` env var
- **gRPC** — Management API for shard control

### Key Interfaces
- `discord.Encoding` — Abstraction over ETF vs JSON encoding
- `state.DB` — Storage backend abstraction (PostgreSQL)

## Environment Variables

- `MULTI_REDIS` — JSON map of Redis addresses to event type filters (required for gateway). Empty array = all events.
- `TOKEN` — Discord bot token
- `ETCD` — etcd endpoints (default: `http://localhost:2379,http://localhost:4001`)
- `SHARDS` — Total shard count
- `START`/`STOP` — Shard range (inclusive/exclusive)
- `INTENTS` — Discord intent set: `default`, `all`, `fast`
- `PSQL` — PostgreSQL address (required; the gateway/state binaries exit if it is unset)
- `PROD` — Enables production logging (JSON/stackdriver vs human-readable)

## Dependencies

Postgres, Redis, and etcd must be available at runtime. (The FoundationDB backend and its client-library requirement have been removed.)
