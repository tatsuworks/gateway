#!/bin/bash

set -euo pipefail
cd "$(dirname "$0")"

VERSION="$(git describe --dirty --always)"
if [[ $VERSION == *-dirty ]]; then
  # We need to ensure the image is loaded again so we give the image a unique
  # name from other images based on this dirty commit.
  VERSION+="-$(head -c 5 < /dev/urandom | base32)"
fi

gateway_uri="rg.fr-par.scw.cloud/tatsu/gateway:$VERSION"
# docker build -t "$gateway_uri" -f Dockerfile.gateway .
# docker push "$gateway_uri" 

state_uri="rg.fr-par.scw.cloud/tatsu/state:$VERSION"
docker build -t "$state_uri" -f Dockerfile.state .
docker push "$state_uri"

echo "New gateway image URI: $gateway_uri"
echo "New state image URI:   $state_uri"
