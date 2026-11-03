# Request middleware and production-oriented flows

## Ownership and startup

A, B and payment share one request policy, not one authentication scheme.

1. `internal/setup.NewRequestRuntime` reads bounded typed configuration and owns
   one telemetry registry and request stack per process.
2. `internal/api.BuildServiceAHandler` and `BuildServiceBHandler` compose HTTP
   routes explicitly. Compatibility constructors use isolated defaults; new
   process startup should use the builders.
3. Payment explicitly installs `RequestStack.UnaryServerInterceptor`; B explicitly
   installs the matching client interceptor. This flow is unary-only. Streaming
   services require their own lifetime/cancellation design and stream interceptors.
4. `pkg/server.ServeHTTPContext` / `ServeGRPCContext` own listeners and bounded
   shutdown. They install no hidden middleware. Payment uses the joined dual
   lifecycle for gRPC and its private HTTP metrics listener.
5. Dependencies close after listeners drain; tracing flushes last, with a deadline.

Implicit metrics listeners/factories have been retired; see
[observability](observability.md) for the migration path.

## Execution order

HTTP routers have one outer context/observation/recovery stack. Business routes
apply capacity and a deadline; Service B additionally installs authentication
and frozen authorization together through `auth.ProtectedRoutes.Handler` before
input decoding or side effects. This preserves JWT/JWKS validation, exact
scope/role matching and explicit client allowlists.

Operational `GET /healthz`, `/readyz` and `/metrics` routes retain context and
recovery but bypass business capacity and permissions. Readiness has its own
one-second dependency budget. Metrics and readiness are not authorization
substitutes and must remain on restricted operational networks.

Payment gRPC applies trace context, observation, recovery, a deadline and business
capacity. Health `Check` and `List` bypass capacity. Identity is enforced by the
exact SPIFFE peers in Envoy, not by unverified gRPC metadata. Service A remains
an unauthenticated **local-only trigger**, not a public production gateway.

## Budgets and failures

| Process variable | Default | Bounds / effect |
| --- | --- | --- |
| `REQUEST_TIMEOUT` | `5s` | `100ms`–`5s`; shorter caller deadlines win |
| `REQUEST_MAX_IN_FLIGHT` | `64` | `1`–`1024` business requests per process |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | Optional HTTPS OTLP/gRPC collector or numeric loopback HTTP collector |
| `PAYMENT_METRICS_ADDRESS` | `127.0.0.1:9090` | Payment-only, loopback HTTP metrics listener |

Compose maps service-specific host variables (`SERVICE_A_REQUEST_TIMEOUT`,
`SERVICE_B_REQUEST_TIMEOUT`, `PAYMENT_REQUEST_TIMEOUT`, and matching
`*_REQUEST_MAX_IN_FLIGHT`) to process variables. It also forwards the optional
OTLP endpoint. A container loopback endpoint refers to that workload namespace,
not the Docker host. No collector is provisioned automatically.

Capacity is non-queuing: HTTP returns 503 with `Retry-After: 1`, while gRPC returns
`ResourceExhausted`. Health requests still run when business capacity is exhausted.
Existing outbound OAuth and payment call budgets remain bounded and use the
request context. No automatic payment retry is added; retry an uncertain result
with the same idempotency key.

Deadlines are cooperative for application work; handlers and repositories must
honor context cancellation. HTTP body reads also receive a transport read deadline.
Middleware does not launch unbounded goroutines to forcibly interrupt handlers.
The existing HTTP write budget is ten seconds; each server drains for at most five
seconds on shutdown.

Recovery never logs panic values. An HTTP panic before final headers produces a
generic 500; a panic after headers aborts the response rather than pretending it
completed successfully. `http.ErrAbortHandler` retains its standard abort semantics.
gRPC panics become sanitized `Internal` errors. HTTP response wrappers expose
`Unwrap` for `http.ResponseController`; use that controller for deadlines/flushing.

## Observation and privacy

`pkg/observability.RequestTelemetry` owns a private Prometheus registry, optional
OTLP exporter and W3C trace propagation. No global registry or metrics listener is
created by a middleware constructor. Trace IDs are generated even without export.
Tracing uses parent-based sampling with a 10% root sample rate; only trace context
is propagated, not baggage or identity headers. Feature collectors join the same
registry; B retains its existing JWKS cache gauges.

All three processes expose `service_requests_total`,
`service_request_duration_seconds` and `service_requests_in_flight`. Labels use
route templates or fixed RPC methods, never raw paths, order IDs, clients or
payloads. HTTP client duration includes response body completion and failures;
server duration includes handler completion. A/B expose `/metrics`; payment
exposes it only on its private metrics listener. The optional `observability`
profile collects all three through a dedicated SPIFFE metrics identity. Runtime,
pool, business capacity, dependency readiness and trace pipeline signals share
each process registry; see [observability](observability.md) for configuration.

Request logs contain locally generated request IDs, trace IDs, operation and
status. Incoming request IDs are not trusted. HTTP response `X-Request-ID` matches
the request logger. Client-correctable denials are not logged as system failures;
5xx/server failures have a sanitized error-level summary. Readiness failures
should be monitored as dependency readiness, not paged as individual business
errors. Tokens, headers, bodies, SQL error strings and panic values are omitted.

## Verification and remaining production work

Local tests cover policy ordering, panic/partial-response behavior, propagation,
capacity bypass for health, caller deadlines, isolated metrics registries and
joined listener shutdown. `scripts/spiffe-smoke.sh` additionally exercises the real
payment/Vitess path and unauthorized workload rejection in an isolated stack.

This execution model does not supply tenant ownership, authoritative pricing,
database per-table privileges, payment reconciliation, a public ingress policy,
fleet-wide rate limits or a collector deployment. Those are explicit deployment
and domain decisions, not generic middleware features.
