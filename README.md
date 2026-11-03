# Go cloud-native microservice blueprint

Single-module Go monorepo for cloud-native microservices.

```
cmd/<service>/        thin process entrypoints (service-a, service-b, payment, vitess-migrate)
internal/<feature>/   business logic, API adapters, auth, middleware, generated protobuf
pkg/                  shared, dependency-light libraries (logging, observability, database, server)
test/                 integration tests and k6 load script
docker/, compose*.yaml, Dockerfile.*   local SPIFFE/Envoy/Vitess stack and per-service images
docs/                 architecture and operations guides
```

Add a service: create `cmd/<name>/main.go` (use `demoruntime.Main` + `setup.NewRequestRuntime`)
and a `Dockerfile.<name>`; `make build` picks up every `cmd/*` automatically.

## SPIFFE mTLS, Envoy and local Vitess

```shell
./scripts/spiffe-up.sh
./scripts/spiffe-smoke.sh
```

The separate `compose.spiffe.yaml` stack provides SPIRE-issued rotating identities,
Envoy mTLS for A → B and B → Vitess, and a private local-development Vitess database.
The default infrastructure/probe mode needs no OAuth credentials. Enable the `apps`
profile with your external IdP configuration to run the unchanged JWT-protected Go
applications. See [the SPIFFE/Envoy guide](docs/spiffe-envoy.md) for deployment,
attestation privileges, isolation, tests and limitations.

## Persistent Vitess order CRUD

```shell
./scripts/vitess-migrate.sh
# After configuring your external IdP:
docker compose -f compose.spiffe.yaml --profile apps up --build --wait
```

`/orders` now supports create, list, get, update and delete with real Vitess
persistence through Envoy. JWT permissions precede every storage operation.
See [the order/driver guide](docs/orders-vitess.md) for migrations, JSON contracts,
pool settings, pagination and CRUD smoke tests.

## Simulated payments over gRPC

The SPIFFE apps profile also connects Service B to a separate payment service over
gRPC, with persistent, idempotent simulated payments. No real money moves. See
[the payment guide](docs/payments.md) for setup, permissions and limitations.

## Service-to-service authentication with an external IdP

```shell
docker compose up --build --wait
./scripts/smoke.sh
```

Configure your external OAuth provider and a reachable VTGate gateway using
`.env.example` before starting standalone Service A and Service B. The SPIFFE
commands above provide the local gateway and migration job. The provider must issue supported signed JWT access
tokens and publish HTTPS JWKS; no IdP or token signer runs in this repository.
Tests use a test-only HTTPS fake issuer and need no external credentials.
See [the complete authentication guide](docs/service-auth.md) for architecture,
environment variables, explicit claim mappings, declarative policies, curl examples,
threat model, production limitations and source ZIP instructions.

## Logging

See [request flows](docs/request-flows.md) for the shared HTTP/gRPC middleware,
explicit composition, capacity/deadline settings and request telemetry.
See [observability](docs/observability.md) for authenticated Prometheus collection,
runtime/database/readiness metrics, trace exporter health and legacy API migration.

Application logging uses `pkg/logging`. Use `logging.FromContext(ctx)` when a
context is available, or `logging.DefaultLogger()` otherwise. Log structured
fields with `Infow`, `Warnw`, and `Errorw`; do not log configuration structs or
credentials.

`LOG_LEVEL` defaults to `INFO`. Logs are JSON on stderr by default; set
`LOG_MODE=development` for console output. Use `logging.WithLogger(ctx, logger)`
to attach a request-specific logger.

## Tests

```shell
make check        # fmt, vet, race tests
```

Tests start their own local servers on ephemeral ports. Vitess-backed integration
tests need the compose stack (see above).

## Roadmap

- OTEL eBPF auto-instrumentation
- NATS messaging
