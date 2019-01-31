#!/bin/bash

VERSION=$(git describe --dirty --broken)

echo $VERSION

go build -ldflags "-X main.Version=$VERSION"
