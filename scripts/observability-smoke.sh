#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
dc() { docker compose -f compose.spiffe.yaml --profile tests --profile observability --profile observability-tests "$@"; }
if [ -n "$(docker compose -f compose.spiffe.yaml --profile apps ps -q service-a service-b)" ]; then
  echo "Run observability fixtures in an isolated project without the apps profile." >&2
  exit 1
fi
python3 scripts/generate-spiffe-envoy.py
dc up -d --build --wait --wait-timeout 240 envoy-a envoy-b envoy-payment envoy-vitess envoy-unauthorized test-backend-a test-backend-b payment prometheus
dc run --rm --no-deps -T collection-probe
dc run --rm --no-deps -T collection-denied-probe
echo "Authenticated metrics collection and collector isolation passed."
