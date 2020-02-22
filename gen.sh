#!/bin/bash

set -euo pipefail
cd "$(dirname "$0")"

# make pushd and popd silent
pushd () { builtin pushd "$@" > /dev/null ; }
popd () { builtin popd > /dev/null ; }

pushd gatewaypb
	protoc -I. --gogofaster_out=plugins=grpc:. ./*.proto
popd
