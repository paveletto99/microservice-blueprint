# External OAuth authentication and declarative authorization

## Scope and setup

The standard Compose stack runs Service A and Service B; the SPIFFE apps profile
also runs the simulated [payment service](payments.md). Your external IdP owns client
registration, credentials, role grants, token issuance and signing-key rotation.
There is no production IdP, private signing key, discovery endpoint or rotation
command here. The HTTPS fake issuer exists exclusively in `*_test.go` files.

Compatibility is vendor-neutral **OAuth2 Client Credentials plus RS256 JWT/JWKS**.
OAuth compliance alone does not guarantee JWT access tokens or public JWKS.
Opaque tokens/introspection, other signing algorithms and interactive login flows
are outside this implementation. The provider must return a Bearer token with a
positive numeric `expires_in` (at most 24 hours), and publish supported public keys.

1. Register a confidential machine client at your IdP; grant the intended scopes
   and resource-specific roles there.
2. Configure the issuer, token endpoint, JWKS endpoint, resource audience and
   claim names according to the provider's documentation. Endpoints are explicit;
   the application does not guess discovery paths or token-supplied URLs.
3. Use `.env.example` as a configuration template. Its values are placeholders,
   not working credentials. Inject real credentials securely and never commit `.env`.
4. Configure a reachable VTGate gateway and apply the additive orders migration.
   The SPIFFE stack handles its local gateway and migration job automatically;
   standalone Compose needs an external `VTGATE_ADDRESS`. See [orders/Vitess](orders-vitess.md).
5. Start the two applications:

```sh
docker compose up --build --wait
./scripts/smoke.sh
```

Requires Docker Compose v2 and curl. Builds/tests use Go 1.27.3. Python 3 is needed
only for ZIP packaging. To build without contacting any IdP, placeholders suffice:

```sh
docker compose --env-file .env.example build
```

Compose runs non-root, read-only containers with loopback-only published ports:
Service A on `127.0.0.1:8081` and Service B on `127.0.0.1:8082`. Its shared bridge
allows outbound HTTPS to the configured external provider. The local A → B demo
uses HTTP only when explicitly permitted; IdP endpoints always require HTTPS.
Do not expose the unauthenticated Service A trigger API publicly. Use
`docker compose down` to stop the applications.

## Configuration

| Variable | Used by | Meaning/default |
| --- | --- | --- |
| OIDC_ISSUER | B | Required exact external HTTPS issuer |
| OIDC_JWKS_URL | B | Required fixed external HTTPS JWKS URL |
| OIDC_TOKEN_URL | A | Required fixed external HTTPS token URL |
| OAUTH_AUDIENCE | B | Required resource audience, not an interactive client audience |
| OAUTH_CLIENT_ID | A | Required registered client ID |
| OAUTH_CLIENT_SECRET | A | Required injected client secret; never logged |
| AUTH_ALLOWED_CLIENTS | B | Required space-separated client IDs after claim mapping |
| OAUTH_AUTH_METHOD | A | `client_secret_basic` (default) or `client_secret_post` |
| OAUTH_SCOPES | A | Optional requested scopes, space-separated |
| OAUTH_RESOURCE | A | Optional provider-specific resource/audience value |
| OAUTH_RESOURCE_PARAMETER | A | `resource` (default) or `audience`; omitted when value is empty |
| SERVICE_B_URL | A | Required when running directly; Compose defaults to http://service-b:8080 |
| ALLOW_HTTP_RESOURCE | A | False in Go; Compose permits only its local HTTP resource connection |
| JWT_TOKEN_TYPE | B | Required matching access-token `typ`; defaults to `at+jwt` |
| JWT_CLIENT_ID_CLAIM | B | `client_id` by default |
| JWT_SCOPE_CLAIM | B | `scope` by default; string or array of strings |
| JWT_ROLES_CLAIM | B | `roles` by default; array of strings |
| JWT_TOKEN_USE_CLAIM | B | Optional access-token discriminator claim |
| JWT_TOKEN_USE_VALUE | B | Expected discriminator value; both discriminator settings are required together |
| JWT_MAX_LIFETIME | B | Maximum exp − iat; default 10m, configurable up to 24h |
| LISTEN_ADDRESS | A/B | Defaults to :8080 |
| LOG_LEVEL / LOG_MODE | A/B | Existing pkg/logging settings |

Issuer/JWKS/token settings and credentials have **no fake-provider defaults**.
Service B validates its explicit allowlist before fetching keys. Configuration
errors reject startup without printing credentials or tokens.

Standard OAuth requests send `grant_type=client_credentials` and optional `scope`.
No `resource`/`audience` parameter is sent unless `OAUTH_RESOURCE` is configured.
Basic auth form-encodes the client ID and secret before Base64 encoding. POST
auth sends credentials only in the HTTPS form body. Authentication method is
configured, never selected by an automatic retry with another credential method.

## Explicit provider claim mapping

Claim settings are literal JSON names or RFC 6901 JSON pointers for nested objects.
For example, a provider exposing `azp`, `scp` and nested resource roles can use:

```text
JWT_CLIENT_ID_CLAIM=azp
JWT_SCOPE_CLAIM=scp
JWT_ROLES_CLAIM=/resource_access/service-b/roles
```

A namespaced JSON claim such as `https://example.test/roles` can be selected by
that literal name. JSON pointers escape `/` as `~1` and `~` as `~0`. There are no
automatic `azp`, `appid`, group or resource-role fallbacks. Configured mappings
are applied only **after signature and registered-claim verification**. Subject
and client ID are both required, but need not be equal: some providers assign a
different subject to the machine principal. `jti` is optional and not replay detection.

If an access token uses `typ=JWT` or another provider type, configure it explicitly.
A generic JWT type alone does not distinguish ID tokens: use the resource API's
audience and, where supported, an explicit marker such as:

```text
JWT_TOKEN_TYPE=JWT
JWT_TOKEN_USE_CLAIM=token_use
JWT_TOKEN_USE_VALUE=access
```

Do not configure an interactive login audience or an ID-token profile for APIs.

## Verification, caches and permissions

Service A obtains tokens from the configured IdP and shares one concurrency-safe
cache across outbound requests. It renews 30 seconds before expiry, capped at
half the token's reported lifetime. Concurrent renewals coalesce; callers can
cancel while waiting. Requests are cloned, and credentials attach only to the
configured resource origin/authority. No redirects or implicit API retries are
introduced. Token errors are generic and never expose provider response bodies.

Service B warms its JWKS cache before readiness and verifies tokens locally. It
accepts RS256 only, checks configured type/issuer/audience, requires exp, iat,
subject and mapped client identity, and validates nbf when present. Application
startup uses five seconds of leeway; constructors limit it to 30 seconds.

JWKS refresh normally runs every five seconds, honoring shorter reasonable cache
headers down to a one-second minimum. Downloads are bounded to five seconds,
64 KiB and 16 keys. Supported RSA keys are 2048–4096 bits. Standard extra metadata
such as x5c/x5t is ignored, not treated as a trust anchor. Nonmatching key types
and algorithms are ignored; malformed supported keys, duplicate supported kids,
private material and invalid signing usage fail refresh. At least one supported
key is required. Token-supplied jku/jwk/x5u, unsupported crit/b64 headers, unsigned
tokens and HS256 are rejected.

Unknown kids trigger a globally rate-limited, singleflight refresh (at most once
per second, including failures). Outages retain last-known-good public keys only
within the configured staleness limit (default ten minutes), while tokens must
still pass expiry checks. Stale/uninitialized caches fail closed. Key publication
overlap and retirement are the external provider's responsibility; the consumer
tests simulate these changes without implementing an IdP administration workflow.
`/metrics` exposes cache age, key count and readiness alongside bounded request
metrics from the [shared request stack](request-flows.md).

Authentication installs verified normalized claims in a private context key.
Authorization does not parse tokens. `Policy` matches exact case-sensitive values,
ANY or ALL within a dimension and AND across configured dimensions. Empty policies,
invalid modes, blank values and duplicate entries are rejected. Registration
deep-copies policy slices. Missing authentication is 401; insufficient permissions
are 403; denied requests never invoke the business handler.

| Route | Scope | Roles | Success |
| --- | --- | --- | --- |
| GET /orders and GET /orders/{id} | ALL orders:read | ANY orders:reader, orders:admin | 200 / 404 |
| POST /orders | ALL orders:write | ANY orders:writer, orders:admin | 201 |
| PUT /orders/{id} | ALL orders:write | ANY orders:writer, orders:admin | 200 / 404 |
| DELETE /orders/{id} | ALL orders:delete | ALL orders:admin, orders:delete | 204 |

Every business route also requires a mapped client in `AUTH_ALLOWED_CLIENTS`.
There is no role hierarchy: orders:admin alone cannot DELETE. Roles/grants are
managed externally, never elevated by client requests. Data now persists in Vitess;
see the order guide for validation and pagination. `/healthz`, `/readyz` and B's
`/metrics` are public; readiness also requires database/schema health.

```sh
curl -i http://127.0.0.1:8082/orders               # 401 without authentication
curl -i http://127.0.0.1:8081/call/orders          # 200 with configured read grants
curl -i -X POST http://127.0.0.1:8081/call/orders -H 'Content-Type: application/json' -d '{"item":"example"}'
# Individual operations use the UUID returned by creation, not a fixed demo ID.
curl -fsS http://127.0.0.1:8082/metrics
```

The basic smoke is read-only. Full persisted CRUD needs deliberate read/write/delete
grants, without changing provider roles through this application:

```sh
./scripts/smoke.sh
ORDERS_CALL_URL=http://127.0.0.1:8081/call/orders python3 scripts/orders-crud-smoke.py
```

## Testing, packaging and limitations

```sh
go test -race ./...
go vet ./...
go build ./...
go test -race -count=10 ./internal/auth ./test/integration
```

Tests use real RSA signatures and test-only HTTPS token/JWKS endpoints; no external
IdP or credentials are needed. They cover policy decisions, malformed tokens,
claims/mappings, supported metadata, cache sharing/renewal/cancellation, unknown
keys, rotation overlap, staleness and endpoint failures. Parser fuzzing and
deterministic timer tests remain. Real-provider smoke checks require your
configuration and credentials and cannot be replaced by fake-provider tests.

Ship source with `git archive --format=zip -o service-auth.zip HEAD`; secrets, `.env`
and build outputs are untracked by design.

For production use HTTPS for resource calls too, restrict networks/ingress, use
secure credential storage and protect Service A's trigger interface. Standard
provider metadata does not authorize a request; only signature-verified configured
claims do. Offline revocation is not immediate, tokens can be replayed, and roles
do not replace tenant/ownership checks. Sender-constrained tokens or introspection
require a separate design. Logs never contain tokens, secrets or configuration structs.
