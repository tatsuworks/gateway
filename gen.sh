#!/bin/bash

set -euo pipefail
cd "$(dirname "$0")"

# make pushd and popd silent
pushd () { builtin pushd "$@" > /dev/null ; }
popd () { builtin popd > /dev/null ; }

GO111MODULE=off go get -u github.com/gogo/protobuf/protoc-gen-gogofaster

pushd gatewaypb
	protoc -I. --gogofaster_out=plugins=grpc:. ./*.proto
popd
