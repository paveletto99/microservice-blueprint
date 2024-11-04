# Go template repository

Use this as a starting point for Go programs, CLI tools, Services, Daemons, Libraries, etc

<https://github.com/sethvargo/go-limiter>
<https://github.com/apple/pkl?tab=readme-ov-file>
<https://sourcegraph.com/github.com/scylladb/scylla-operator/-/blob/pkg/probeserver/scylladbapistatus/prober.go?L117:10-117:16>
<https://packagemain.tech/p/graceful-shutdowns-k8s-go>

## TODO

- database
- add otel
- add prom metrics
- nats
- scylla
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
    "crypto/tls"
    "log"
    "net"

    "github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
    "github.com/spiffe/go-spiffe/v2/workloadapi"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
)

func main() {
    ctx := context.Background()

    // Create a Workload API client
    client, err := workloadapi.New(ctx, workloadapi.WithAddr("unix:///tmp/spire-agent.sock"))
    if err != nil {
        log.Fatalf("Unable to create Workload API client: %v", err)
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
        log.Fatalf("Failed to listen: %v", err)
    }

    log.Println("Starting gRPC server...")
    if err := grpcServer.Serve(lis); err != nil {
        log.Fatalf("Failed to serve: %v", err)
    }
}


```

client

```go

package main

import (
    "context"
    "crypto/tls"
    "log"

    "github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
    "github.com/spiffe/go-spiffe/v2/workloadapi"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
)

func main() {
    ctx := context.Background()

    // Create a Workload API client
    client, err := workloadapi.New(ctx, workloadapi.WithAddr("unix:///tmp/spire-agent.sock"))
    if err != nil {
        log.Fatalf("Unable to create Workload API client: %v", err)
    }
    defer client.Close()

    // Create TLS configuration
    tlsConfig := tlsconfig.MTLSClientConfig(client, tlsconfig.AuthorizeAny())

    // Dial gRPC server with the TLS configuration
    conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
    if err != nil {
        log.Fatalf("Failed to dial server: %v", err)
    }
    defer conn.Close()

    // Call your services here
    // client := pb.NewYourServiceClient(conn)
    // response, err := client.YourMethod(ctx, &pb.YourRequest{})
}


```
