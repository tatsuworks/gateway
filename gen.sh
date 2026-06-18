#!/bin/bash

set -euo pipefail
cd "$(dirname "$0")"

# make pushd and popd silent
pushd () { builtin pushd "$@" > /dev/null ; }
popd () { builtin popd > /dev/null ; }

# Install the codegen plugin into GOBIN (module-aware; the old
# `GO111MODULE=off go get -u` form was removed in recent Go toolchains).
go install github.com/gogo/protobuf/protoc-gen-gogofaster@v1.3.2

# protoc resolves --gogofaster_out via the plugin binary on PATH.
export PATH="$(go env GOPATH)/bin:$PATH"

pushd gatewaypb
	protoc -I. --gogofaster_out=plugins=grpc:. ./*.proto
popd
