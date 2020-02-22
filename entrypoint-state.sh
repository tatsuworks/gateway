#!/bin/bash

exec /state \
	--prod="$PROD" \
	--psql="$PSQL" \
	--addr="$ADDR"
