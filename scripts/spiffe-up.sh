#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
python3 scripts/generate-spiffe-envoy.py
docker compose -f compose.spiffe.yaml up --build --wait --wait-timeout 240
curl --silent --show-error --fail --max-time 10 "http://127.0.0.1:${SPIFFE_HTTP_PORT:-18081}/__spiffe_probe"
echo "SPIRE, Envoy mTLS and the private Vitess gateway are ready."
