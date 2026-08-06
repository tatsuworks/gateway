#!/bin/bash

set -euo pipefail
cd "$(dirname "$0")"

VERSION="$(git describe --dirty --always)"
if [[ $VERSION == *-dirty ]]; then
  # We need to ensure the image is loaded again so we give the image a unique
  # name from other images based on this dirty commit.
  VERSION+="-$(head -c 5 < /dev/urandom | base32)"
fi

# Cluster nodes are amd64, so always build for linux/amd64 — a plain
# `docker build` on an Apple Silicon Mac produces an arm64 image that
# crashloops on k8s with "exec format error" (see the 2026-08-05 whirlwind
# staging incident, tatsuworks/whirlwind#56).
readonly BUILD_PLATFORM="linux/amd64"

# Refuse to push an image with the wrong architecture.
assert_amd64() {
  local image="$1" arch
  arch="$(docker image inspect "$image" --format '{{.Architecture}}')"
  if [[ "$arch" != "amd64" ]]; then
    echo "Built image architecture for $image is '$arch', expected amd64 — not pushing." >&2
    exit 1
  fi
}

# gateway_uri="gcr.io/tatsu-production/gateway:$VERSION"
gateway_uri="6222o0k9.gra7.container-registry.ovh.net/tatsu/gateway:$VERSION-release"
docker build --platform "$BUILD_PLATFORM" -t "$gateway_uri" -f Dockerfile.gateway .
assert_amd64 "$gateway_uri"
docker push "$gateway_uri"

# state_uri="gcr.io/tatsu-production/state:$VERSION"
state_uri="6222o0k9.gra7.container-registry.ovh.net/tatsu/state:$VERSION-release"

docker build --platform "$BUILD_PLATFORM" -t "$state_uri" -f Dockerfile.state .
assert_amd64 "$state_uri"
docker push "$state_uri"

echo "New gateway image URI: $gateway_uri"
echo "New state image URI:   $state_uri"
