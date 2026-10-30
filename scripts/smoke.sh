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
status "${EXPECT_POST_STATUS:-201}" POST http://127.0.0.1:8081/call/orders
status "${EXPECT_DELETE_STATUS:-403}" DELETE http://127.0.0.1:8081/call/orders/demo-1
echo "External OAuth client credentials and authorization smoke checks passed."
