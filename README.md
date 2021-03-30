# gateway

Service for multiplexing many Discord websockets on top of any number of backends.
Consists of 2 components: Gateway & State Cache

NOTE: This currently only supports Tatsu's specific use case. More backends will be written in the future
as time permits.

A custom ETF parser was written from the ground up. During peak traffic, gateway uses ~4 cores for 720 shards.

State is stored in [foundationdb](https://www.foundationdb.org/) and accessed via the State Cache (cmd/state).

Events are pushed to [redis](https://redis.io) using `RPUSH`. The content is the `d` key of the event encoded as ETF.

## Common Dependencies

1. Ensure you have Go 1.13 or higher.
1. Install [foundationdb](https://www.foundationdb.org/download/) (both the server and clients package).
1. Install [redis](https://redis.io).

## Setting up the Gateway

1. Add a variable in `cmd/gateway/main.go` named `Token` that contains your token.
1. D `cmd/gateway/main.go` named `Token` that contains your token.
1. Enable modules `export GO111MODULE=on`
1. Run `go build` in `cmd/gateway`
1. To run, do `./gateway` in `cmd/gateway`
1. To only push certain event types to the event queue, set the `WHITELIST_EVENTS` environment variable e.g `WHITELIST_EVENTS=MESSAGE_CREATE,MESSAGE_REACTION_ADD`

## Setting up the State

1. Run `go build` in `cmd/state`
1. To run, do `./state` in `cmd/state`
