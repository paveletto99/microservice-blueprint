# SPIFFE/SPIRE, Envoy mTLS and local Vitess

## Scope and quick start

This is an additional, complete Compose stack, **not an overlay on compose.yaml**.
It keeps all Go JWT verification and scope/role policies unchanged. Certificate
handling belongs to Envoy; SPIRE supplies workload X.509-SVIDs and trust bundles
through its native SDS interface. No Go application configures SPIFFE TLS.

Requires Docker Compose v2, Python 3 and curl on a trusted Linux Docker host.
Rootful Docker 29/cgroups v2 is tested; rootless/user-namespace variants are not.

```sh
./scripts/spiffe-up.sh
./scripts/spiffe-smoke.sh
```

The default stack starts infrastructure/proxies and Vitess only, so no external
IdP credentials are needed to prove mTLS. The smoke script adds temporary test
fixtures, checks identity and isolation, runs MySQL and gRPC queries, and waits
for SDS certificate renewal (normally tens of seconds; bounded at 100 seconds).

To run the actual A/B applications with the **existing external OAuth/JWKS settings**:

```sh
# Configure your external provider as described in docs/service-auth.md first.
docker compose -f compose.spiffe.yaml --profile apps up --build --wait --wait-timeout 240
curl -i http://127.0.0.1:18081/call/orders
curl -i -X POST http://127.0.0.1:18081/call/orders -H 'Content-Type: application/json' -d '{"item":"example"}'
# Read/update/delete use the UUID returned by creation; see docs/orders-vitess.md.
```

Do not run `apps` and the smoke-test fixtures together: the test backend occupies
the same private application port as Service B. Stop the test stack before enabling
`apps`. This mode requires real external provider credentials; transport-only tests
do not replace a real JWT/IdP deployment smoke check.

## Connections and identities

```text
service-a → localhost:18000 / Envoy A == SPIFFE mTLS ==> Envoy B → localhost:8080 / service-b
                                                           |
                                    localhost:15991 / 15993 / Envoy B
                                                           |
                                                      SPIFFE mTLS
                                                           |
                                                     Envoy Vitess
                                                           |
                                                private Vitess backend
```

| Envoy workload | SPIFFE ID | Attested UID | Allowed peer |
| --- | --- | --- | --- |
| A | spiffe://service-auth.test/service-a | 10001 | Expects B's identity |
| B | spiffe://service-auth.test/service-b | 10002 | Accepts A; expects vtgate for database calls |
| Vitess gateway | spiffe://service-auth.test/vtgate | 10003 | Accepts B only |
| Test-only unauthorized proxy | spiffe://service-auth.test/unauthorized | 10004 | Has a valid SVID but is rejected by B and Vitess |

Each proxy receives only its attested identity. Both TLS directions validate the
SPIRE bundle **and an exact URI SAN**, not merely any certificate from the trust
domain. Inbound listeners require a client certificate. TLS minimum and maximum
are explicitly TLS 1.3, avoiding Envoy's upstream TLS 1.2 default maximum.
Secrets never appear in Envoy config: certificates, private keys and bundles
arrive over SDS and update new connections without application restarts.

SPIFFE proves workload identity and encrypts transport. JWTs still prove the
configured issuer/audience and permissions in Go. This independent conjunction
does **not** turn ordinary JWTs into OAuth sender-constrained/mTLS-bound tokens.
No certificate role hierarchy, implicit scopes, token introspection, or JWT
validation in Envoy is added.

## No proxy bypass

Applications share their own Envoy's network namespace and bind **127.0.0.1:8080**.
They have no host-published application ports and no direct SPIRE socket mounts.
Service A's resource origin is explicitly `http://127.0.0.1:18000`; this plaintext
hop remains inside its workload namespace. The cross-workload hop is mTLS.

Only A's demo ingress is published, on **127.0.0.1:18081** (override SPIFFE_HTTP_PORT
if needed). B's 8443 listener and database gateway's 8444/8445 listeners remain on
the mesh network. Envoy admin is loopback-only on 9901; SPIRE control/API sockets
are not exposed to the host or applications. Forged x-spiffe-id and
x-forwarded-client-cert headers are sanitized, never used for authentication.

The sole `/__spiffe_probe` route is a non-sensitive transport health response. It
still requires the correct mTLS identity at B, but deliberately does not require
a JWT or a running Go backend. All business paths are forwarded to the unchanged
Go handlers and their JWT policies when `apps` is enabled.

Vitess attaches **only to an internal database network**; only Envoy Vitess joins
both database and mesh networks. Raw query, MySQL and administration ports cannot
be reached from mesh peers or host-published sockets. SQL authorization remains
Vitess/MySQL's responsibility; allowing B's identity is not per-table authorization.

## SPIRE bootstrap, attestation and storage

Pinned runtime versions are SPIRE 1.15.5, Envoy 1.40.2 and Vitess 24.0.5/MySQL 8.4.
Base images use verified manifest digests. Generated Envoy JSON is maintained by:

```sh
python3 scripts/generate-spiffe-envoy.py
python3 -m unittest discover -s test/spiffe
```

Do not hand-edit generated files. Restart proxies after configuration changes.
SPIRE handles credential renewal dynamically, without restarting proxies.

Startup order: volume permissions → SPIRE server → authenticated bootstrap and
registration → SPIRE agent → Envoy readiness/SDS credentials. Vitess starts in
parallel; its gateway waits for both agent and database query health.

The non-root SPIRE server owns its state/socket volumes as UID 1000. A narrowly
scoped initialization job changes metadata only on those named volumes. A bootstrap
job uses the protected server Unix API to create a short-lived, one-use join token
and export the trusted public bundle. It captures output in memory and writes
0600 files to a separate private volume; tokens are never echoed or passed as
arguments. The agent uses `join_token_file` and a pinned bootstrap bundle, **not
insecure_bootstrap**. Existing registrations must match their selectors, parent,
non-admin permissions and TTL or bootstrap rejects them.

Workload attestation requires **both distinct Unix UIDs and observed Docker labels**.
Docker labels are retrieved from the Docker runtime using the workload PID/cgroup;
request headers do not influence identity. Only Envoy containers mount the agent
socket. Default workload SVID TTL is 60 seconds for this demo, agent TTL one hour,
CA TTL 24 hours. These short lifetimes facilitate real rotation tests, not a
production lifetime recommendation.

**Privileged infrastructure warning:** the agent has host PID/cgroup visibility,
SYS_PTRACE and access to `/var/run/docker.sock`. A read-only filesystem mount
does **not** make the Docker API read-only. Treat the agent, Docker administrators,
bootstrap job and SPIRE state as trusted infrastructure. No application or Envoy
receives Docker API access. Production should use an appropriate platform-native
attestor and tighter host isolation rather than blindly copying these privileges.

SPIRE server/agent state, bootstrap credentials and Vitess data use named volumes.
Normal restarts reuse matching state; they do not regenerate or expose keys.
Missing/inconsistent partial bootstrap state fails closed rather than automatically
disabling trust validation. To stop while preserving state:

```sh
docker compose -f compose.spiffe.yaml down
```

For a **destructive demo reset**, `down --volumes` deletes this stack's SPIRE keys,
bootstrap identity and **all Vitess data**. Never use it against valuable data.

## Vitess and test coverage

The pinned official vttestserver image provides one unsharded `commerce` keyspace,
VTGate/VTCombo and local MySQL. Data persists under the named Vitess volume. It is
a local-development database, not a production HA/sharded Vitess deployment.

From B's namespace, Envoy exposes:

- `127.0.0.1:15991`: existing Vitess gRPC driver → gateway mTLS port 8444 → private VTGate gRPC.
- `127.0.0.1:15993`: MySQL protocol → gateway mTLS port 8445 → private VTGate MySQL listener.

The gRPC probe uses the repository's existing Vitess driver through Envoy and also
exercises persisted CRUD, same-value updates and migration idempotence. A race-enabled
HTTP/JWT test uses the real repository over this gateway. A second MySQL probe
exercises the other listener. Wrong SPIFFE identities and plaintext attempts fail.
The `apps` profile now includes the additive orders migration and wires B's protected
CRUD handlers to the database. See [the order/driver guide](orders-vitess.md).
Existing data is never reset automatically.

Smoke also verifies a loopback-only backend through authenticated proxy forwarding,
rejects forged identity headers, checks raw B/Vitess/underlying MySQL/admin port
bypasses from a mesh peer, and observes changed public certificate serial numbers
while traffic continues. Expired certificates prevent new authenticated TLS
handshakes. Existing connections are not instantaneously revoked; connection
lifetimes are bounded to 30 seconds and credential updates apply to new connections.
SPIRE outages therefore are not equivalent to immediate revocation of all open
connections. Liveness alone is not proof that a specific mTLS route is ready.

```sh
go test -race ./...
go vet ./...
go build ./...
python3 -m unittest discover -s test/spiffe
./scripts/spiffe-smoke.sh
```

Existing JWT tests still use only a test-local HTTPS issuer and keep their 401/403,
scope/role/client, cache, expiry and signature coverage. No fake production IdP is
reintroduced. Full `apps` smoke with external JWT credentials remains operator-run.
Transport test fixtures exist only in the `tests` profile and must not be deployed
as production backends.

Source ZIP packaging includes these configurations, generators, Dockerfiles and
test source but excludes all volume state, credentials, SVID/private keys, binaries
and generated dependency artifacts. The stack is a trusted-host demonstration;
protect the local trigger ingress, keep production resource/ownership policy
checks, and use hardened SPIRE/Vitess deployments for production.
