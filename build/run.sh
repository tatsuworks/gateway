#!/bin/bash

set -euo pipefail
cd "$(dirname "$0")"

docker build -t gateway-build .

PROJECT_ROOT="$(git rev-parse --show-toplevel)"
export PROJECT_ROOT

docker run --rm \
	-u "$(id -u "${USER}"):$(id -g "${USER}")" \
	-v "/etc/passwd:/etc/passwd" \
	-v "$HOME:$HOME" \
	gateway-build \
	bash -c "/build.sh"
