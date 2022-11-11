# gateway

Service for multiplexing many Discord websockets on top of any number of backends.
Consists of 2 components: Gateway & State Cache

NOTE: This currently only supports Tatsu's specific use case.

A custom ETF parser was written from the ground up. During peak traffic, gateway uses ~4 cores for 720 shards.

State can be stored in either [foundationdb](https://www.foundationdb.org/) or [postgresql](https://www.postgresql.org/) and accessed via the State Cache (cmd/state).

Events are pushed to [redis](https://redis.io) using `RPUSH`. The content is the `d` key of the event encoded as ETF.

## Common Dependencies

1. Ensure you have Go 1.18 or higher.
2. Install [foundationdb](https://apple.github.io/foundationdb/downloads.html).
    - Client package is required for building Gateway/State.
    - Server package is only required when running using fdb.
3. Install [redis](https://redis.io).

## Setting up the Gateway

1. Add a variable in `cmd/gateway/main.go` named `Token` that contains your token.
2. D `cmd/gateway/main.go` named `Token` that contains your token.
3. Enable modules `export GO111MODULE=on`
4. Run `go build` in `cmd/gateway`
5. To run, do `./gateway` in `cmd/gateway`
6. To only push certain event types to the event queue, set the `WHITELIST_EVENTS` environment variable e.g `WHITELIST_EVENTS=MESSAGE_CREATE,MESSAGE_REACTION_ADD`

## Setting up the State

1. Run `go build` in `cmd/state`
2. To run, do `./state` in `cmd/state`
