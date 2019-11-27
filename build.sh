#!/bin/bash

set -euo pipefail
cd "$(dirname "$0")"

docker build -t rg.fr-par.scw.cloud/tatsu/gateway -f Dockerfile.gateway .
docker push rg.fr-par.scw.cloud/tatsu/gateway

docker build -t rg.fr-par.scw.cloud/tatsu/state -f Dockerfile.state .
docker push rg.fr-par.scw.cloud/tatsu/state
