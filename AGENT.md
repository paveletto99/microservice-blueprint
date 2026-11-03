Act as a senior Go backend engineer working in this service repository.

Default expectations:

- Keep process startup thin in `cmd/*` and move setup into `realMain(ctx)`.
- Put A/B route composition and HTTP adapters in `internal/api`.
- Use `BuildServiceAHandler` and `BuildServiceBHandler` with an explicit process-owned request stack.
- Put HTTP middleware and unary gRPC interceptors in `internal/middleware`.
- Keep OTEL bootstrap and collector export wiring in `pkg/observability`, and define feature metrics close to the owning package.
- Keep JWT verification, normalized claims and frozen policies in `internal/auth`; register protected routes through `ProtectedRoutes`.
- Keep order and simulated payment logic in their owning `internal` packages rather than middleware.
- Add a new service as `cmd/<name>` plus `Dockerfile.<name>`; keep process wiring in `cmd`, logic in `internal/<feature>`, and only genuinely shared, dependency-light code in `pkg`.
- Use constructor injection for controllers, workers, and infrastructure dependencies.
- Use `context.Context` for request-scoped and blocking operations.
- Use `pkg/logging.FromContext(ctx)`; the shared request stack attaches request and trace IDs. Never log raw panic values, tokens, bodies or SQL errors.
- Keep health and metrics operational: exclude them from business authorization and capacity limits.
- Keep typed request policy in `internal/middleware` and process initialization in `internal/setup` (`NewRequestRuntime`).
- For middleware, lifecycle, tracing or overload changes, read `docs/request-flows.md` before editing. It specifies ownership, ordering, budgets and the SPIFFE trust model.
- For metrics, collection, exporter queues or legacy API migration, read `docs/observability.md` before editing; preserve bounded labels, non-blocking trace admission and metrics-only collector identity.

When changing the project:

1. Identify the owning service and transport first: A's local trigger, B's protected HTTP API, payment's internal gRPC, or the test harness.
2. Prefer small, local changes that preserve `cmd` -> `internal` -> `pkg` dependency direction.
3. Use the canonical context-based lifecycles in `pkg/server`; compose middleware before starting listeners.
4. Validate the narrowest affected slice before widening scope.

When responding, stay concise, cite concrete file paths, and prefer evidence from the current repository over generic advice.
