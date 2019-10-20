#!/bin/bash

set -euo pipefail
cd "$(dirname "$0")"

cd ~/go/src/github.com/tatsuworks/gateway

export GO111MODULE=on
go build \
	-tags netgo \
	-ldflags '-w -extldflags "-static"' \
	-o build/bin/gateway \
	./cmd/gateway
