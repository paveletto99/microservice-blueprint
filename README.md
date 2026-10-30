# Go template repository

Use this as a starting point for Go programs, CLI tools, Services, Daemons, Libraries, etc

## Service-to-service authentication with an external IdP

```shell
docker compose up --build --wait
./scripts/smoke.sh
```

Configure your external OAuth provider using `.env.example` before starting
Service A and Service B. The provider must issue supported signed JWT access
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
- spiffe/spire

## SPIFFE NOTES


Register your gRPC services with SPIRE to get their identities (SPIFFE IDs). For example:


```shell
./spire-server entry create -spiffeID spiffe://example.org/serviceA -parentID spiffe://example.org/agent -selector unix:uid:1001
./spire-server entry create -spiffeID spiffe://example.org/serviceB -parentID spiffe://example.org/agent -selector unix:uid:1002

```


server

```go
package main

import (
    "context"
    "net"

    "github.com/paveletto99/microservice-blueprint/pkg/logging"
    "github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
    "github.com/spiffe/go-spiffe/v2/workloadapi"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
)

func main() {
    logger := logging.DefaultLogger()
    defer logger.Sync()
    ctx := logging.WithLogger(context.Background(), logger)

    // Create a Workload API client
    client, err := workloadapi.New(ctx, workloadapi.WithAddr("unix:///tmp/spire-agent.sock"))
    if err != nil {
        logger.Fatalw("unable to create Workload API client", "error", err)
    }
    defer client.Close()

    // Create TLS configuration
    tlsConfig := tlsconfig.MTLSServerConfig(client, tlsconfig.AuthorizeAny())

    // Create a new gRPC server with the TLS configuration
    grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))

    // Register your services here
    // pb.RegisterYourServiceServer(grpcServer, &yourService{})

    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        logger.Fatalw("failed to listen", "error", err)
    }

    logger.Info("starting gRPC server")
    if err := grpcServer.Serve(lis); err != nil {
        logger.Fatalw("failed to serve", "error", err)
    }
}


```

client

```go

package main

import (
    "context"

    "github.com/paveletto99/microservice-blueprint/pkg/logging"
    "github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
    "github.com/spiffe/go-spiffe/v2/workloadapi"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
)

func main() {
    logger := logging.DefaultLogger()
    defer logger.Sync()
    ctx := logging.WithLogger(context.Background(), logger)

    // Create a Workload API client
    client, err := workloadapi.New(ctx, workloadapi.WithAddr("unix:///tmp/spire-agent.sock"))
    if err != nil {
        logger.Fatalw("unable to create Workload API client", "error", err)
    }
    defer client.Close()

    // Create TLS configuration
    tlsConfig := tlsconfig.MTLSClientConfig(client, tlsconfig.AuthorizeAny())

    // Dial gRPC server with the TLS configuration
    conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
    if err != nil {
        logger.Fatalw("failed to dial server", "error", err)
    }
    defer conn.Close()

    // Call your services here
    // client := pb.NewYourServiceClient(conn)
    // response, err := client.YourMethod(ctx, &pb.YourRequest{})
}


```
