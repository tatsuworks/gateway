#!/bin/bash

POD_ID=${HOSTNAME##*-}
START=$((POD_ID * SHARDS_PER_POD))
STOP=$(((POD_ID+1) * SHARDS_PER_POD))

exec /gateway \
	--token="$TOKEN" \
	--name="$NAME" \
	--prod="$PROD" \
	--redis="$REDIS" \
	--etcd="$ETCD" \
	--pprof="$PPROF" \
	--addr="$ADDR" \
	--shards="$SHARDS" \
	--start="$START" \
	--stop="$STOP" \
	--intents="$INTENTS" \
	--psqlAddr="$PSQL" \
	--podId="$POD_ID"
