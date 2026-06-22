# gateway

Service for multiplexing many Discord websockets on top of any number of backends.
Consists of 2 components: Gateway & State Cache

NOTE: This currently only supports Tatsu's specific use case. More backends will be written in the future
as time permits.

A custom ETF parser was written from the ground up. During peak traffic, gateway uses ~4 cores for 720 shards.

State is stored in [Postgres](https://www.postgresql.org/) and accessed via the State Cache (cmd/state). Pass the Postgres address with `-psqlAddr` (gateway) or `-psql` (state); it is required.

Events are pushed to [redis](https://redis.io) using `RPUSH`. The content is the `d` key of the event encoded as ETF.

## Common Dependencies

1. Ensure you have Go 1.13 or higher.
2. Install [Postgres](https://www.postgresql.org/download/).
3. Install [redis](https://redis.io).

## Setting up the Gateway

1. Add a variable in `cmd/gateway/main.go` named `Token` that contains your token.
2. Enable modules `export GO111MODULE=on`
3. Run `go build` in `cmd/gateway`
4. To run, do `./gateway` in `cmd/gateway`

### Redis Configuration

The gateway requires the `MULTI_REDIS` environment variable to be set. This allows you to configure one or more Redis instances with optional per-instance event filtering.

**Format:** `{"redis_address": ["EVENT_TYPE1", "EVENT_TYPE2"], ...}`

**Examples:**

Single Redis instance, no event filtering (all events sent):
```bash
export MULTI_REDIS='{"localhost:6379":[]}'
```

Single Redis instance with event filtering:
```bash
export MULTI_REDIS='{"localhost:6379":["MESSAGE_CREATE","MESSAGE_UPDATE","MESSAGE_DELETE"]}'
```

Multiple Redis instances with different event filters:
```bash
export MULTI_REDIS='{"redis1.example.com:6379":["MESSAGE_CREATE","MESSAGE_UPDATE"],"redis2.example.com:6379":["GUILD_CREATE","GUILD_UPDATE"]}'
```

## Setting up the State

1. Run `go build` in `cmd/state`
2. To run, do `./state` in `cmd/state`
