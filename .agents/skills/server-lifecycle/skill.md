# Server Setup Handlers And Lifecycle

Use this skill when you change startup wiring, long-lived dependencies, or handler construction in `cmd/*` and `internal/routes/*`.

## Arguments

- `entrypoint`: the binary in `cmd/` that owns the change
- `dependencies`: database, cache, key manager, limiter, auth, renderer, or observability pieces being added or removed
- `shutdown`: what must be closed or stopped cleanly

## Instructions

Keep `main()` thin. Follow the repo pattern from `cmd/server/main.go`: create a signal-aware root context, attach the logger, and move all failure-returning setup into `realMain(ctx)`.

Build long-lived dependencies once at process startup. Load config first, then observability, cache, database, signing keys, rate limiter, and auth providers. Defer cleanup immediately after successful creation in the same function.

Inject those dependencies into the route builder, and create controllers from the route layer rather than inside handlers. Handler methods should return `http.Handler` and use request context for request-scoped values only.

Do not create network clients, database handles, renderers, or heavyweight crypto objects per request. Do not read environment variables directly from controllers or middleware when the value belongs in typed config.

If you introduce a new dependency, verify three things before finishing: startup ordering is explicit, shutdown is graceful, and the dependency is only threaded to the controllers that actually need it.
