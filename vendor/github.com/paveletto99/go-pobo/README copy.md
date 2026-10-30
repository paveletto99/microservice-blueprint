# New Project Template Example

This folder is a copyable starter scaffold for a new Go service that follows the same high-level shape as this repository.

The scaffold assumes Go 1.22+ so it can use `log/slog` and send logs, metrics, and traces to an OpenTelemetry collector over OTLP.

The files use a `.tmpl` suffix so they do not participate in the current workspace build. To use them:

1. Copy this folder into a new repository or service directory.
2. Rename `*.tmpl` files to their real names.
3. Replace `example.com/my-project` with your module path.
4. Fill in the TODOs for auth, rate limiting, database, and deployment details.

## Layout

```text
examples/new-project-template/
├── AGENT.md
├── .env.example
├── README.md
├── builders/build.yaml.tmpl
├── builders/service.dockerfile.tmpl
├── go.mod.tmpl
├── cmd/adminapi/main.go.tmpl
├── cmd/server/main.go.tmpl
├── cmd/worker/main.go.tmpl
├── internal/envstest/harness.go.tmpl
├── internal/routes/adminapi.go.tmpl
├── internal/routes/server.go.tmpl
├── internal/routes/worker.go.tmpl
├── pkg/cache/cache.go.tmpl
├── pkg/cookiestore/cookiestore.go.tmpl
├── pkg/config/adminapi_config.go.tmpl
├── pkg/config/server_config.go.tmpl
├── pkg/config/worker_config.go.tmpl
├── pkg/controller/context.go.tmpl
├── pkg/controller/admin/controller.go.tmpl
├── pkg/controller/example/controller.go.tmpl
├── pkg/controller/example/metrics.go.tmpl
├── pkg/controller/example/controller_test.go.tmpl
├── pkg/controller/middleware/observability.go.tmpl
├── pkg/controller/middleware/require_permission.go.tmpl
├── pkg/controller/middleware/request_id.go.tmpl
├── pkg/controller/middleware/session.go.tmpl
├── pkg/controller/worker/controller.go.tmpl
├── pkg/database/database.go.tmpl
├── pkg/observability/observability.go.tmpl
├── pkg/pagination/pagination.go.tmpl
├── pkg/render/renderer.go.tmpl
├── pkg/rbac/rbac.go.tmpl
├── pkg/worker/example/worker.go.tmpl
├── scripts/build.tmpl
├── terraform/locals.tf.tmpl
├── terraform/main.tf.tmpl
├── terraform/service_server.tf.tmpl
├── terraform/variables.tf.tmpl
└── docs/development.md.tmpl
```

## What This Shows

- a thin `cmd/server` entrypoint with `realMain(ctx)`
- a thin `cmd/adminapi` entrypoint for JSON/admin surfaces
- a thin `cmd/worker` entrypoint for scheduled or internal jobs
- an `AGENT.md` with repo-local coding guidance for future changes
- route assembly in `internal/routes`
- constructor-injected controllers in `pkg/controller`
- context helpers for sessions and permissions in `pkg/controller/context`
- a sample request ID middleware in `pkg/controller/middleware`
- collector-first OTLP setup for logs, metrics, and traces in `pkg/observability`
- observability and session middleware seams in `pkg/controller/middleware`
- cookie store, RBAC, pagination, and observability packages under `pkg/`
- example-local metrics definitions close to the owning controller
- a reusable worker stub in `pkg/worker`
- a minimal `internal/envstest` harness and example test
- typed environment config in `pkg/config`
- extracted local build, Cloud Build, and Docker packaging seams
- explicit infrastructure stubs for database, cache, render, and Terraform

This is intentionally minimal. It shows the seams to start from scratch without pretending to be a finished service.

## Topic Packages

This scaffold now includes starter files for the same cross-cutting topics that show up repeatedly in this repository:

- observability
- RBAC
- metrics
- pagination
- cookie and session handling

The server route example wires observability and cookie-backed sessions directly so those files show a real integration path instead of standing alone as unused placeholders.

## Telemetry Defaults

- Each binary initializes OTEL once at process startup and exports logs, metrics, and traces through OTLP.
- For host-based local runs, keep `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`.
- For containerized runs on a shared network, point the same variable at `http://otel-collector:4318`.
- The example handlers use `slog` with request context so emitted logs can be correlated with active traces in the collector.

## Build And Package

This scaffold follows the same packaging split as this repo: build the Go binary into `bin/`, package it with a thin service Dockerfile, and optionally wrap remote builds through `scripts/build` plus `builders/build.yaml`.

Local binary build:

```sh
mkdir -p bin
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o ./bin/server ./cmd/server
```

Local Docker package:

```sh
docker build --file builders/service.dockerfile --build-arg SERVICE=server --tag my-project/server:dev .
```

Cloud Build wrapper:

```sh
PROJECT_ID=my-project-id SERVICE=server TAG=dev ./scripts/build
```
