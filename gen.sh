#!/bin/bash

pushd () { command pushd "$@" > /dev/null ; }
popd () { command popd "$@" > /dev/null ; }

pushd gatewaypb
	protoc -I. --gogofaster_out=plugins=grpc:. *.proto
popd
