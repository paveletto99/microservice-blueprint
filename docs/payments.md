# Simulated HTTP-to-gRPC payments

The SPIFFE apps profile adds Service B → payment gRPC to the existing Service A → B
HTTP example. Payments are **simulated records only**: no provider is called and
no money moves. Orders contain no price, so the caller-supplied amount is not an
authoritative charge. Do not use this example to process real payments.

## Start

Configure the external IdP and its client credentials as described in
[service-auth.md](service-auth.md). Grant Service A `orders:write` and
`payments:write` scopes and the `orders:writer` and `payments:writer` roles.
Then run:

```sh
docker compose -f compose.spiffe.yaml --profile apps up --build --wait --wait-timeout 240
curl -i -X POST http://127.0.0.1:18081/call/orders \
  -H 'Content-Type: application/json' -d '{"item":"example"}'
# Replace ORDER_ID with the ID returned above; reuse the same key when retrying.
curl -i -X POST http://127.0.0.1:18081/call/orders/ORDER_ID/payments \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: example-payment-1' \
  -d '{"amount_minor":1234,"currency":"USD"}'
```

Service A is an unauthenticated local demo trigger and must not be exposed
publicly. Service B requires an allowed client, `payments:write` scope and either
`payments:writer` or `payments:admin` role. The order must exist. HTTP input is
bounded to 4 KiB, and accepts only a positive integer `amount_minor` (maximum
1,000,000,000,000) and three uppercase ASCII currency letters. Currency minor-unit
definitions and real currency codes are not validated.

The required `Idempotency-Key` is 1–128 printable ASCII characters (excluding
commas). Keys are scoped to the authenticated client ID and stored only as a
SHA-256 digest. First creation returns 201 and a UUID; an identical replay returns
200 and the same UUID; reuse with a different order, amount or currency returns
409. Requests can be retried with the same key after a timeout; no automatic
payment retries are performed. Replays still require the order to exist. Order
deletion and payment creation are not a distributed transaction; records remain
after an order is deleted. There is no tenant isolation, refund, reconciliation,
or payment processor integration.

## Storage, identity and operations

The payment process owns `simulated_payments` and `payment_schema_migrations` in
the demo's shared Vitess keyspace. The payment migration is additive and separately
versioned, so the existing order migration remains compatible. The apps profile
starts the migration before payment, and payment readiness checks its schema.
This demonstrates logical ownership, not per-table database authorization: Vitess
SQL permissions require a separate production design.

Service B connects to its loopback Envoy on `127.0.0.1:15051`; Envoy authenticates
the payment Envoy's exact SPIFFE ID and uses mTLS. The payment Envoy accepts only
Service B's identity and forwards to the loopback-only gRPC server. Payment's
separate Vitess listener accepts only the payment identity. Plaintext is restricted
to each workload's shared network namespace, not the mesh. The app-level gRPC
server trusts that boundary and does not verify JWTs independently. Service B
readiness checks payment gRPC health and its dependency health.

The shared [request stack](request-flows.md) supplies explicit HTTP/gRPC recovery,
deadlines, capacity limits, metrics and trace propagation. Payment metrics are
available only at its loopback listener on `127.0.0.1:9090`.

The previous `payment.Payment/Create` float RPC is retained only to return
`Unimplemented`; use `payment.v1.PaymentService/CreatePayment`. The latter is an
internal RPC and expects a digest scoped by the authenticated HTTP caller. The
old RPC's placeholder success was unsafe to treat as a payment.

The standalone `compose.yaml` remains the existing A/B example and does not
enable payments. `PAYMENT_GRPC_ADDRESS`, when configured on B, must be a numeric
loopback address pointing to an authenticated local proxy; it enables the route
and makes payment readiness mandatory. Do not point it to a local proxy without
an equivalent identity and transport policy.

Run `go test -race ./internal/payment ./internal/client ./test/integration` for
local tests. `./scripts/spiffe-smoke.sh` additionally exercises payment Vitess
concurrency, the live gRPC path and rejection of an unauthorized SPIFFE identity;
it uses the test profile and does not require an external IdP. The optional
`VTGATE_TEST_ADDRESS` tests must target an isolated development database.
