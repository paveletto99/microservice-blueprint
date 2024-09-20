/*===========================================================================*\

\*===========================================================================*/

package service

import (
	"context"

	"github.com/paveletto99/microservice-blueprint/internal/serverenv"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	payment "github.com/paveletto99/microservice-blueprint/pkg/api/payment/v1"
)

// Server is the admin server.
type Client struct {
}

type Config struct {
}

// NewServer makes a new admin console server.
func NewClient(ctx context.Context, config *Config, env *serverenv.ServerEnv) (*Client, error) {
	// if env.Database() == nil {
	// 	return nil, fmt.Errorf("missing Database in server env")
	// }
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.NewClient("http:/localhost:8080", opts...)
	if err != nil {
		logging.Info(ctx, "It is fine, this is not a complete example.")
	}

	defer conn.Close()

	paymentClient := payment.NewPaymentClient(conn)
	_, err = paymentClient.Create(ctx, &payment.CreatePaymentRequest{Price: 23})
	if err != nil {
		logging.Info(ctx, "Don't worry, we don't expect to see it is working.")
	}

	return &Client{}, nil
}
