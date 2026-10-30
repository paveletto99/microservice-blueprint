#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
status() {
  expected=$1
  method=$2
  endpoint=$3
  actual=$(curl --silent --show-error --max-time 10 --output /dev/null --write-out '%{http_code}' --request "$method" "$endpoint")
  if [ "$actual" != "$expected" ]; then
    echo "Smoke failure: $method $endpoint returned $actual, expected $expected" >&2
    exit 1
  fi
}
status 200 GET http://127.0.0.1:8082/readyz
status 200 GET http://127.0.0.1:8081/readyz
status 401 GET http://127.0.0.1:8082/orders
status 200 GET http://127.0.0.1:8081/call/orders
echo "External OAuth readiness and authenticated read smoke checks passed (no database rows mutated)."
echo "For full CRUD with delete privileges: python3 scripts/orders-crud-smoke.py"
