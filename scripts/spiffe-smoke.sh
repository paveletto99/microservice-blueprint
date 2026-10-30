#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
dc() { docker compose -f compose.spiffe.yaml --profile tests "$@"; }
# Test backend is intentionally separate from the JWT applications; never run both profiles together.
if [ -n "$(dc ps -q service-b)" ]; then
  echo "Run transport smoke without the apps profile (the test backend uses its loopback port)." >&2
  exit 1
fi
dc up -d --wait --wait-timeout 120 envoy-a envoy-b envoy-vitess envoy-unauthorized test-backend-b
# Safe readiness GET only: a replaced proxy can briefly retain a previous DNS address.
curl --silent --show-error --fail --max-time 5 --retry 10 --retry-all-errors --retry-delay 1 "http://127.0.0.1:${SPIFFE_HTTP_PORT:-18081}/__spiffe_probe"
dc run --rm --no-deps -T probe-allowed
backend=$(dc ps -q vitess)
VITESS_BACKEND_IP=$(docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$backend")
export VITESS_BACKEND_IP
dc run --rm --no-deps -T probe-denied
result=$(dc run --rm --no-deps -T db-mysql-probe)
if [ "$result" != 1 ]; then echo "Vitess MySQL mTLS query failed" >&2; exit 1; fi
dc build db-grpc-probe
dc run --rm --no-deps -T db-grpc-probe
dc run --rm --no-deps -T db-grpc-probe --crud
dc build orders-crud-test
dc run --rm --no-deps -T orders-crud-test
if dc run --rm --no-deps -T db-denied-probe >/dev/null 2>&1; then
  echo "Unauthorized SPIFFE identity reached Vitess" >&2
  exit 1
fi
if dc run --rm --no-deps -T --entrypoint mysql db-mysql-probe \
  --protocol=TCP --host=envoy-vitess --port=8445 --user=root --connect-timeout=3 --execute='SELECT 1' >/dev/null 2>&1; then
  echo "Plaintext database gateway connection unexpectedly succeeded" >&2
  exit 1
fi
if [ "${SPIFFE_CHECK_RENEWAL:-true}" = true ]; then
  dc run --rm --no-deps -T probe-allowed renewal
fi
echo "SPIFFE identity, proxy isolation, Vitess queries and renewal smoke checks passed."
