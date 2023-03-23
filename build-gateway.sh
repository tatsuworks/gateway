#!/bin/bash

set -euo pipefail
cd "$(dirname "$0")"

VERSION="$(git describe --dirty --always)"
if [[ $VERSION == *-dirty ]]; then
  # We need to ensure the image is loaded again so we give the image a unique
  # name from other images based on this dirty commit.
  VERSION+="-$(head -c 5 < /dev/urandom | base32)"
fi

# gateway_uri="gcr.io/tatsu-production/gateway:$VERSION"
gateway_uri="6222o0k9.gra7.container-registry.ovh.net/tatsu/gateway:$VERSION"
docker build -t "$gateway_uri" -f Dockerfile.gateway .
docker push "$gateway_uri"

echo "New gateway image URI: $gateway_uri"
