# Metrics collection and tracing health

## Authenticated collection

Configure the external IdP as described in [service-auth.md](service-auth.md), then:

```sh
docker compose -f compose.spiffe.yaml --profile apps --profile observability up --build --wait --wait-timeout 240
docker compose -f compose.spiffe.yaml --profile observability exec -T prometheus \
  /bin/promtool query instant http://127.0.0.1:9090 up
```

The optional `observability` profile runs pinned Prometheus LTS 3.13.5 and a
dedicated Envoy with `spiffe://service-auth.test/metrics-collector` (UID 10006).
Prometheus scrapes A, B and payment through loopback listeners 18010–18012 in its
own namespace. Each mesh hop uses HTTP/3 over QUIC (TLS 1.3, UDP 8448) and
validates the exact server identity. The application's metrics-only listener on UDP 8448 accepts only the collector's
identity and forwards only `GET /metrics`. It cannot forward business routes.

Application listeners remain private; neither the collector API nor UI has a
host-published port. Prometheus binds its API to collector loopback only. Use
`promtool` through Compose exec, or design an explicitly authenticated operational
gateway for production access. Prometheus receives no SPIRE socket or private
certificate files; its Envoy owns SDS credentials.

Scrapes have a three-second timeout, 2 MiB response bound, 10,000-sample limit and
20-label limit. Redirects and supplied timestamps are disabled. This local profile
uses bounded tmpfs storage, two-hour / 64 MiB retention and a 512 MiB memory limit;
data is ephemeral on restart. Persistent storage, fleet discovery and remote
retention belong in a production collector deployment.

`scripts/observability-smoke.sh` runs a transport test profile without an IdP. A
and B use explicitly test-only metrics fixtures; payment is the real application.
The test checks all three scrape targets, real payment telemetry ingestion,
collector rejection on business routes, forged-header rejection and raw-port
bypass rejection. Run it in a fresh isolated Compose project, never alongside the
actual A/B apps. It provisions a development database and preserves volumes unless
you explicitly clean up that test project.

## Signals

- **Requests:** `service_requests_total`, duration in seconds, and total transport
  in-flight work. HTTP client observations end at EOF, known-length completion or
  body close, not at response headers. Read errors report failure; early close
  reports an aborted result; multiple close/end signals complete exactly once.
  Always close response bodies. A 200 header followed by a truncated body remains
  status 200 with outcome failure—query outcomes, not only 5xx codes.
- **Capacity:** `service_business_request_capacity`,
  `service_business_requests_in_flight`, and `service_capacity_rejections_total`.
  Operational traffic does not consume business permits.
- **Dependencies:** `service_dependency_ready{dependency="..."}` checks fixed
  OAuth/JWKS/database/payment dependencies with a one-second budget per probe.
  Probes must honor cancellation and run during scraping. A successful gRPC health
  RPC can carry `NOT_SERVING`; this gauge records dependency readiness independently
  of transport success. No error messages or addresses become labels.
- **Database:** SQL pool connection capacity and open/in-use/idle counts, waits,
  cumulative wait seconds and connection eviction counters. Collection uses one
  `sql.DB.Stats()` snapshot and performs no SQL queries.
- **Process:** standard Go runtime and process collectors in each process-owned
  registry. Metrics contain no payloads, credentials or connection strings.

Request and trace metrics retain their fixed service labels; all collected series
also have trusted Prometheus `job` and `instance` labels. Route templates and fixed
RPC methods are used instead of user IDs, raw URLs or query parameters.

Useful PromQL:

```promql
sum by (job, protocol) (rate(service_requests_total{outcome=~"failure|aborted"}[5m]))
service_business_requests_in_flight / service_business_request_capacity
database_pool_connections{state="in_use"} / ignoring(state) database_pool_connection_capacity
rate(database_pool_wait_duration_seconds_total[5m])
service_dependency_ready == 0
```

Example warning rules for scrape failures, dependency readiness, trace export
failures/drops and business saturation are loaded from
`docker/observability/alerts.yaml`. They do not provision Alertmanager or paging.
Operational readiness failures are separate from individual business-request errors.

## Bounded trace export

`OTEL_EXPORTER_OTLP_ENDPOINT` configures **trace export**, not metric push. Metrics
are pulled by Prometheus. Empty means tracing export is disabled, while local
trace context and request correlation still work. HTTPS is required except for a
numeric loopback HTTP collector inside the same namespace.

Sampled spans have a bounded admission queue (default 512 spans, batches of 128),
one-second scheduling and two-second export deadlines. Admission is non-blocking:
queue saturation drops spans, not application requests. SDK batching and shutdown
remain in use; a thin admission/export wrapper reports the following signals:

- `service_trace_export_enabled`, queue capacity/pending and export in-flight.
- `service_trace_export_batches_total{result="success|failure"}` and last successful
  export timestamp. Results are measured after the OTLP exporter's own retries.
- Successfully exported spans, spans in failed batches, admission drops by fixed
  reason, and shutdown failures. A partial export failure counts the batch as
  failed rather than claiming every span was accepted.

Queue pending includes admitted spans waiting in a batch but excludes an active
export attempt. In-memory trace buffers are not durable. Collector failures are
counted and sanitized before reaching the SDK error handler. Force-flush waits
for its admitted-span snapshot on the bounded SDK schedule; canceled flushes do
not insert control markers or consume request queue slots. Shutdown flushes the
SDK directly within the caller's deadline. Tests exercise body-read failures,
early close, unavailable exporters, queue saturation and repeated canceled flushes.

## Legacy retirement

`ServeMetricsIfPrometheus` and `NewPrometheusMetricsFactory` now return
`ErrLegacyMetricsRetired`. They no longer open a hard-coded/global listener or call
`os.Exit`. Old compatibility metric/factory types remain source-compatible but
inert; migrate to `RequestTelemetry` and feature-owned collectors. The removed
quic-go server no longer opens port 2223; HTTP/3 now terminates in Envoy. The old experimental
`internal/metrics.New` is deprecated and remains only for existing explicit-registry
callers. No new flow uses it.
