# Persistent orders with Vitess

## Start and migrate

Orders use the existing Vitess `database/sql` gRPC driver. In the SPIFFE stack,
Service B connects to its local Envoy at `127.0.0.1:15991`, targeting
`commerce@primary`. Envoy authenticates B to the private Vitess gateway using
SPIRE SVIDs; Go does not configure certificate files or bypass TLS verification.

```sh
./scripts/spiffe-up.sh
./scripts/vitess-migrate.sh
# With your existing external OAuth/JWKS configuration:
docker compose -f compose.spiffe.yaml --profile apps up --build --wait --wait-timeout 240
```

The `apps` profile includes `orders-migrate` and makes B wait for successful
migration. The explicit migration script also works on existing development
volumes without starting JWT applications or requiring OAuth credentials.
Standalone `compose.yaml` requires a reachable, appropriately protected VTGate
address; its container loopback is not the SPIFFE sidecar namespace. Prefer the
SPIFFE stack, or set `VTGATE_ADDRESS` to your existing gateway/proxy.

Schema version 1 adds only `orders` and `service_auth_schema_migrations`. It never
drops/replaces data or resets volumes. SQL is embedded from
`internal/orders/migrations/0001_orders.sql`; its SHA-256 is recorded and checked.
An incompatible existing orders table, changed checksum or newer database version
rejects migration rather than silently rewriting the schema. MySQL DDL implicitly
commits; idempotent creation and column checks recover interrupted first runs.
Do not modify applied migrations. Future changes require a new version.

`docker compose down` preserves named-volume data; `down --volumes` is destructive
and is never run automatically by migration or CRUD.

## Driver and repository

`pkg/database.OpenVitess` validates the address, primary target and pool settings,
opens a bounded pool and checks connectivity with a context deadline. The legacy
`NewFromEnv` wrapper now honors explicit VTGate settings instead of hardcoding an
unqualified target. PostgreSQL-style legacy settings are not used by the new path.

| Setting | Default | Bounds |
| --- | --- | --- |
| VTGATE_ADDRESS | 127.0.0.1:15991 | hostname/IP and valid TCP port |
| VTGATE_TARGET | commerce@primary | Named keyspace, primary tablet target |
| VTGATE_MAX_OPEN_CONNS | 10 | 1–64 |
| VTGATE_MAX_IDLE_CONNS | 5 | 0–max open |
| VTGATE_TIMEOUT | 5s | 100ms–30s startup check |
| VTGATE_CONN_LIFETIME | 5m | 1s–1h |
| VTGATE_CONN_IDLE_TIME | 1m | 1s–connection lifetime |

SPIFFE Compose fixes address/target to the authenticated local gateway. Repository
calls have a five-second bound while preserving shorter caller deadlines and
cancellation. Pools close on shutdown. B refuses startup with an unavailable or
unmigrated schema; `/readyz` checks both JWKS freshness and database/schema health.
`/healthz` remains process liveness.

`internal/orders.Repository` uses fixed SQL with named bind parameters such as
`:id`/`:item`; input is never interpolated into SQL. IDs are server-generated
canonical UUIDs stored with an ASCII binary primary key. Items are trimmed,
valid UTF-8, nonblank, at most 255 Unicode characters, and contain no controls.
Updates lock the row in a transaction before writing, so same-value updates
remain successful rather than returning a false 404 from zero changed rows.
Deletes use affected-row counts; missing records return typed `ErrNotFound`.

This is a single-keyspace demo. Orders are globally scoped; tenancy/ownership,
sharding/vindexes and production SQL-user privileges require separate designs.
UUID ordering is deterministic but is not creation-time order.

## HTTP contract and permissions

Authentication and frozen policy checks run before input processing and every
repository call. All routes require the configured client allowlist plus:

| Route | Scope | Roles | Result |
| --- | --- | --- | --- |
| POST /orders | orders:write | ANY orders:writer, orders:admin | 201 |
| GET /orders | orders:read | ANY orders:reader, orders:admin | 200 |
| GET /orders/{id} | orders:read | ANY orders:reader, orders:admin | 200 or 404 |
| PUT /orders/{id} | orders:write | ANY orders:writer, orders:admin | 200 or 404 |
| DELETE /orders/{id} | orders:delete | ALL orders:admin, orders:delete | 204 or 404 |

No implicit delete privilege exists for orders:admin. Invalid tokens remain 401;
insufficient permissions remain 403. Invalid identifiers/pagination or malformed
JSON are 400; non-JSON writes are 415; bodies over 4 KiB are 413. Dependency failures
are generic 503 responses. SQL errors and bind values are not exposed in logs or
responses.

POST/PUT require exactly `{"item":"example"}`. Unknown, duplicate and trailing
JSON fields/values are rejected; clients cannot assign IDs. Responses contain
`{"caller":"...","order":{"id":"...","item":"..."}}`. Creation sets Location
to `/orders/{id}`; Service A rewrites it to `/call/orders/{id}` and forwards all
CRUD methods with its existing shared OAuth token cache.

Lists contain caller, an orders array and optional `next_after`. `limit` defaults
to 20 and is limited to 1–100; `after` is a canonical UUID cursor. Results use a
bounded limit-plus-one query, ordered by ID. Unknown/duplicate query parameters
are rejected. Pagination is not a snapshot across concurrent writes.

```sh
curl -i -X POST http://127.0.0.1:18081/call/orders \
  -H 'Content-Type: application/json' -d '{"item":"example"}'
curl -i 'http://127.0.0.1:18081/call/orders?limit=20'
# Use the returned UUID for individual operations:
curl -i http://127.0.0.1:18081/call/orders/RETURNED_UUID
curl -i -X PUT http://127.0.0.1:18081/call/orders/RETURNED_UUID \
  -H 'Content-Type: application/json' -d '{"item":"updated"}'
curl -i -X DELETE http://127.0.0.1:18081/call/orders/RETURNED_UUID
```

## Tests

```sh
go test -race ./...
go vet ./...
go build ./...
./scripts/spiffe-smoke.sh
# Needs an external IdP granting read/write and both delete roles and scope:
python3 scripts/orders-crud-smoke.py
```

Normal HTTP/JWT tests use a test-local repository fake: denied and invalid
requests must not call storage. Docker smoke executes repository CRUD and a
race-enabled real Vitess HTTP/JWT test in B's namespace. Actual SQL crosses
SPIFFE Envoy mTLS; the HTTP issuer is a test-local TLS fake, not a production IdP.
Tests cover persisted data, SQL-punctuation bind values, same-value updates,
limits, missing records, migration idempotence and preservation of unique fixtures.
They never reset volumes and clean up only generated order IDs.

For direct isolated development integration, set `VTGATE_TEST_ADDRESS` and optional
`VTGATE_TEST_TARGET`; otherwise the real Vitess test is explicitly skipped.
The external-provider CRUD script creates a unique fixture and attempts cleanup,
but cannot delete it without delete grants. It defaults to the SPIFFE A ingress;
set `ORDERS_CALL_URL=http://127.0.0.1:8081/call/orders` for standalone Compose.
The basic OAuth smoke script is read-only.
