# gateway

Service for multiplexing many Discord websockets on top of any number of backends.
Consists of 2 components: Gateway & State Cache

_**NOTE:** This currently only supports Tatsu's specific use case._

A custom ETF parser was written from the ground up. During peak traffic, gateway uses ~4 cores for 720 shards.

## Common Dependencies

- Ensure you have Go 1.18 or higher.
- Install [foundationdb](https://apple.github.io/foundationdb/downloads.html).
   - Client package is required for building Gateway/State.
   - Server package is only required when running using fdb.
- Install a database.
  - [postgresql](https://www.postgresql.org/) - in use and maintained.
  - [foundationdb](https://www.foundationdb.org/) - no longer used and maintained.

## [Gateway](https://github.com/tatsuworks/gateway/tree/master/cmd/gateway)

Gateway ingests and parses all events received from Discord, then caches and forwards them to clients.

### Dependencies
- A database (postgres or fdb)
- A method to push events (redis or grpc)
- An [etcd](https://github.com/etcd-io/etcd) server

### Pushing Events
We currently support two methods of pushing events.
- [redis](https://redis.io) - Contents pushed using `RPUSH`. The content is the `d` key of the event encoded as ETF.
- [grpc](https://grpc.io/) - Contents pushed to a [gRPC server](https://github.com/Aericio/grpc-go-to-node/tree/master/node-server) with protobuf.

## Setting up the Gateway

1. Enable modules `export GO111MODULE=on`
2. Navigate to `cmd/gateway`
3. Build `go build`
4. Start `./gateway -token "Bot xxx" -psql "postgres://xxx"`
5. To only push certain event types to the event queue, set the `WHITELIST_EVENTS` environment variable
   - e.g. `WHITELIST_EVENTS=MESSAGE_CREATE,MESSAGE_REACTION_ADD`

## [State](https://github.com/tatsuworks/gateway/tree/master/cmd/state)

The data cached by Gateway can be accessed through State via REST API.

### Dependencies
- A database (postgres or fdb)

### Setting up State
1. Enable modules `export GO111MODULE=on`
2. Navigate to `cmd/gateway`
3. Build  `go build`
4. Start `./state -psql "postgres://xxx"`
