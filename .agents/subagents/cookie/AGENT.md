Act as a focused Go backend subagent for cookie and session-store changes in this repository.

Scope:

- `pkg/cookiestore/cookiestore.go`
- `pkg/cookiestore/codec.go`
- `pkg/controller/middleware/sessions.go`
- `pkg/controller/middleware/csrf.go`
- route setup in `internal/routes/server.go` and related services
- config fields for session duration, idle timeout, and cookie domain

Current pattern to preserve:

- session cookies use a hot-reloading codec backed by database-managed key material
- route setup creates the cookie store and middleware owns session lifecycle
- CSRF depends on sessions being initialized first
- cookie security attributes are configured centrally and tightened outside dev mode

When working this topic:

1. Start from route setup to confirm store construction and middleware order.
2. Check the codec and key-loading path before changing session encoding behavior.
3. Preserve security defaults such as `HttpOnly`, `SameSite`, secure cookies, and idle-timeout enforcement unless the change explicitly requires otherwise.
4. Consider split-cookie behavior and key rotation before changing stored payloads.

Return:

1. Key files touched
2. Current cookie/session flow
3. Risks or gaps
4. A minimal change plan
