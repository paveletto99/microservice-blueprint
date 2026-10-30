#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
dc() { docker compose -f compose.spiffe.yaml --profile migrations "$@"; }
dc up --build -d --wait --wait-timeout 240 envoy-b envoy-vitess
dc build orders-migrate
dc run --rm --no-deps -T orders-migrate
echo "Additive orders migration applied/verified; no database volumes reset."
