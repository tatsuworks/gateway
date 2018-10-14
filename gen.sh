#!/bin/bash

# make pushd and popd silent
pushd () { command pushd "$@" > /dev/null ; }
popd () { command popd "$@" > /dev/null ; }

pushd pb
    protoc --gogofaster_out=plugins=grpc:. *.proto
popd
