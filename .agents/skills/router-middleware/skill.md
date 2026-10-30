# Routers And Middlewares

Use this skill when adding endpoints or changing middleware order in `internal/routes/*` or `pkg/controller/middleware/*`.

## Arguments

- `surface`: `server`, `apiserver`, `adminapi`, or another HTTP surface
- `auth_mode`: public, session, API key, membership, MFA, or admin
- `response_kind`: HTML, JSON, redirect, or file
- `rate_limit_key`: user, API key, realm, or none

## Instructions

Start from the smallest router that directly owns the behavior. Build on Gorilla Mux subrouters, not ad hoc conditionals inside handlers.

Install only truly global middleware at the router root. Add auth, RBAC, firewall, chaff, CSRF, locale, and rate limiting on the specific subrouter that needs them. Keep `/health` on a lighter stack whenever possible.

Preserve ordering rules already documented by the repo. Request ID, trace ID, logger, recovery, and observability belong early. In the API server, chaff runs before rate limiting so padding traffic does not consume quota. In the UI server, sessions and CSRF belong in the shared web stack before protected subroutes.

Handler methods should return `http.Handler`; middleware should remain composable and side effects should be request-scoped. If a feature needs form-based `PUT` or `PATCH`, keep the outer `MutateMethod` wrapper instead of inventing route-local workarounds.

When a route only differs by middleware, prefer a new subrouter over branching inside the controller.
