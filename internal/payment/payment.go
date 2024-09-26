package payment

import (
	"context"

	p "github.com/paveletto99/microservice-blueprint/internal/pb/payment"
	"github.com/paveletto99/microservice-blueprint/internal/serverenv"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
)

// Compile time assert that this server implements the required grpc interface.
var _ p.PaymentServer = (*Server)(nil)

// NewServer builds a new FederationServer.
func NewServer(env *serverenv.ServerEnv, config *Config) p.PaymentServer {
	return &Server{
		env: env,
		// db:        database.New(env.Database()),
		// publishdb: publishdb.New(env.Database()),
		config: config,
	}
}

type Server struct {
	p.UnimplementedPaymentServer
	env *serverenv.ServerEnv
	// db        *database.FederationOutDB
	// publishdb *publishdb.PublishDB
	config *Config
}

// Create implements the PaymentServer Create endpoint.
func (s Server) Create(ctx context.Context, req *p.CreatePaymentRequest) (*p.CreatePaymentResponse, error) {
	logger := logging.FromNamedContext(ctx, "payment.Create")

	ctx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()
	// response, err := s.fetch(ctx, req, s.publishdb.IterateExposures, publishmodel.TruncateWindow(time.Now(), s.config.TruncateWindow)) // Don't fetch the current window, which isn't complete yet.
	// if err != nil {
	// 	stats.Record(ctx, mFetchFailed.M(1))
	logger.Error("failed to fetch", "error")
	// 	return nil, errors.New("internal error")
	// }
	response := p.CreatePaymentResponse{BillId: 0}

	return &response, nil
}
