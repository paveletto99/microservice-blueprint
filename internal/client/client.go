/*===========================================================================*\

\*===========================================================================*/

package service

import (
	"context"

	"github.com/paveletto99/microservice-blueprint/internal/serverenv"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	p "github.com/paveletto99/microservice-blueprint/internal/pb/payment"
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
		logging.FromContext(ctx).Warnw("example payment client connection failed", "error", err)
	}

	defer conn.Close()

	paymentClient := p.NewPaymentClient(conn)
	_, err = paymentClient.Create(ctx, &p.CreatePaymentRequest{Price: 23})
	if err != nil {
		logging.FromContext(ctx).Warnw("example payment request failed", "error", err)
	}

	return &Client{}, nil
}
