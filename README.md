# gateway

service for multiplexing many Discord websockets on top of any number of backends.

## Setting up the Gateway
1. Clone the gateway
2. `export GO111MODULE=on`
3. `go mod vendor`
4. Edit `cmd/gateway/main.go` where it says `m := manager.New(...)`
5. Change `Token` to `"Bot YOUR_TOKEN"`
6. Change `400` to the amount of Bot shards you want
7. Change `m.Start(5)` to the amount of shards you want to start up
7. `go build cmd/gateway/main.go`
8. `./main` 
