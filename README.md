# Go template repository

Use this as a starting point for Go programs, CLI tools, Services, Daemons, Libraries, etc

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

<https://github.com/sethvargo/go-limiter>
<https://github.com/apple/pkl?tab=readme-ov-file>
<https://sourcegraph.com/github.com/scylladb/scylla-operator/-/blob/pkg/probeserver/scylladbapistatus/prober.go?L117:10-117:16>
<https://packagemain.tech/p/graceful-shutdowns-k8s-go>
<https://github.com/uber-go/mock/tree/main>

## Logging

Application logging uses `pkg/logging`. Use `logging.FromContext(ctx)` when a
context is available, or `logging.DefaultLogger()` otherwise. Log structured
fields with `Infow`, `Warnw`, and `Errorw`; do not log configuration structs or
credentials.

`LOG_LEVEL` defaults to `INFO`. Logs are JSON on stderr by default; set
`LOG_MODE=development` for console output. Use `logging.WithLogger(ctx, logger)`
to attach a request-specific logger.

## Tests

```shell
go test -race ./...
```

HTTP/3 and metrics tests start their own local servers on ephemeral ports.
The Scylla integration test is opt-in and requires an **isolated test keyspace**
with the `mutant_data` table. It inserts a unique fixture, verifies it, and cleans
it up:

```shell
SCYLLA_TEST_HOST=127.0.0.1:9042 SCYLLA_TEST_KEYSPACE=blueprint_test \
  go test -race ./internal/scylla -run TestClusterIntegration
```

## TODO

- database - vitess/scylla
- add otel - ebpf auto-instrumentation
- add prom metrics - add otel metrics
- nats

The old incomplete SPIRE Docker scaffolding and direct-Go sketches have been
replaced by the maintained Envoy/SPIRE stack described in [docs/spiffe-envoy.md](docs/spiffe-envoy.md).
