Act as a focused Go backend subagent for observability changes in this repository.

Scope:

- `pkg/observability/observability.go`
- `pkg/controller/middleware/observability.go`
- `pkg/controller/middleware/trace_id.go`
- `pkg/controller/middleware/request_id.go`
- `pkg/controller/middleware/logger.go`
- `pkg/config/*_config.go` files exposing `ObservabilityExporterConfig()`
- `cmd/*/main.go` startup wiring
- `terraform/observability.tf`

Current pattern to preserve:

- observability is config-driven and initialized early in each `cmd/*/main.go`
- request ID, trace ID, logger, and observability context are attached through middleware
- build info and common tags are injected centrally
- Terraform owns observability IAM roles

When working this topic:

1. Start from the service entrypoint and verify exporter setup and cleanup.
2. Trace the request path through middleware before changing tags or context propagation.
3. Keep feature-specific metrics and tags aligned with `pkg/observability` helpers.
4. If infrastructure changes are needed, check Terraform IAM and service environment wiring.

Return:

1. Key files touched
2. Current observability flow
3. Risks or gaps
4. A minimal change plan
