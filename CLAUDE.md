# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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
- `internal/state/` — `state.DB` interface (~93 methods) abstracting storage backends
- `internal/state/db/statefdb/` — FoundationDB implementation (primary)
- `internal/state/db/statepsql/` — PostgreSQL implementation (fallback)
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
- `state.DB` — Storage backend abstraction (FoundationDB or PostgreSQL)

## Environment Variables

- `MULTI_REDIS` — JSON map of Redis addresses to event type filters (required for gateway). Empty array = all events.
- `TOKEN` — Discord bot token
- `ETCD` — etcd endpoints (default: `http://localhost:2379,http://localhost:4001`)
- `SHARDS` — Total shard count
- `START`/`STOP` — Shard range (inclusive/exclusive)
- `INTENTS` — Discord intent set: `default`, `all`, `fast`
- `PSQL` — PostgreSQL address (fallback; otherwise uses FoundationDB)
- `PROD` — Enables production logging (JSON/stackdriver vs human-readable)

## Dependencies

Requires FoundationDB client libraries (v6.2.27) installed on the system. Redis and etcd must be available at runtime.
