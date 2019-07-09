# gateway

service for multiplexing many Discord websockets on top of any number of backends.

NOTE: This currently only supports Tatsu's specific use case. More backends will be written in the future
as time permits.

State is stored in [foundationdb](https://www.foundationdb.org/) and accessed via a separate [service](https://github.com/tatsuworks/state).

Events are pushed to [redis](https://redis.io) using `RPUSH`. The content is the `d` key of the event encoded as ETF.

Websocket compression still TODO.

## Setting up the Gateway

1. Install [foundationdb](https://www.foundationdb.org/download/) (both the server and clients package).
1. Install redis.
1. Ensure you have Go 1.12 or higher.
1. Add a variable in `cmd/gateway/main.go` named `Token` that contains your token.
1. Enable modules `export GO111MODULE=on`
1. Run `go build` in `cmd/gateway`
