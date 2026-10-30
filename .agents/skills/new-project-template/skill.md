# New Project Template

Use this skill when bootstrapping a new service or sibling project that should feel like a natural extension of this repository.

## Arguments

- `services`: binaries you need under `cmd/`
- `shared_packages`: config, database, cache, render, auth, or other common packages to extract early
- `http_surfaces`: UI, API, admin, worker, or none
- `terraform_scope`: minimal service only or full shared infrastructure stack

## Instructions

Start with the same spine used here: thin binaries in `cmd/`, route assembly in `internal/routes/`, and reusable infrastructure in `pkg/`. Create one typed config package first, then wire startup through `realMain(ctx)`.

For HTTP services, define controllers as structs with constructor-injected dependencies and register them from the route layer. Use middleware for request-scoped concerns such as auth, request IDs, locale, and rate limiting. Keep health endpoints minimal.

For infrastructure, begin with shared locals plus one service definition. Give the service a dedicated service account, explicit secret access, and environment layering from shared locals to service overrides.

Favor the boring first version: one route builder, one renderer, one database handle, one limiter, and clear cleanup on shutdown. Expand only after the core dependency flow is stable.

Use the playbooks in `docs/playbooks/feature-development-wiki.md` and `docs/playbooks/new-project-template.md` as the canonical examples.
