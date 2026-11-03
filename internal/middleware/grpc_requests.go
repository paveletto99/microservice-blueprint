package middleware

import (
	"context"
	"errors"

	"github.com/paveletto99/microservice-blueprint/pkg/logging"
	"github.com/paveletto99/microservice-blueprint/pkg/observability"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func incomingTraceCarrier(ctx context.Context) propagation.MapCarrier {
	md, _ := metadata.FromIncomingContext(ctx)
	carrier := propagation.MapCarrier{}
	for _, key := range []string{"traceparent", "tracestate"} {
		if values := md.Get(key); len(values) == 1 {
			carrier[key] = values[0]
		}
	}
	return carrier
}

func injectOutgoingTrace(ctx context.Context) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	carrier := propagation.MapCarrier{}
	observability.TracePropagator().Inject(ctx, carrier)
	for key, value := range carrier {
		md.Set(key, value)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

func operationalRPC(method string) bool {
	return method == "/grpc.health.v1.Health/Check" || method == "/grpc.health.v1.Health/List"
}

func grpcOutcome(err error) string {
	switch status.Code(err) {
	case codes.OK:
		return "success"
	case codes.Canceled:
		return "canceled"
	case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists, codes.PermissionDenied, codes.Unauthenticated, codes.FailedPrecondition, codes.OutOfRange, codes.Unimplemented:
		return "rejected"
	default:
		return "failure"
	}
}

// UnaryServerInterceptor adds tracing, safe panic recovery, deadlines and business capacity.
// It does not authenticate metadata: the payment server trusts its private SPIFFE proxy path.
func (s *RequestStack) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (response any, err error) {
		ctx = observability.TracePropagator().Extract(ctx, incomingTraceCarrier(ctx))
		ctx, observation := s.telemetry.StartRequest(ctx, "grpc", info.FullMethod, trace.SpanKindServer)
		ctx = requestLoggerContext(ctx)
		defer func() {
			panicked := recover() != nil
			if panicked {
				response = nil
				err = status.Error(codes.Internal, "internal server error")
			}
			outcome := grpcOutcome(err)
			observation.End(info.FullMethod, status.Code(err).String(), outcome)
			logger := logging.FromContext(ctx)
			if outcome == "failure" && (!operationalRPC(info.FullMethod) || panicked) {
				logger.Errorw("grpc request failed", "operation", info.FullMethod, "status", status.Code(err).String())
			} else {
				logger.Infow("grpc request completed", "operation", info.FullMethod, "status", status.Code(err).String())
			}
		}()
		ctx, cancel := context.WithTimeout(ctx, s.policy.Timeout)
		defer cancel()
		if ctx.Err() != nil {
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		if !operationalRPC(info.FullMethod) {
			select {
			case s.permits <- struct{}{}:
				s.telemetry.BusinessStarted()
				defer func() { s.telemetry.BusinessCompleted(); <-s.permits }()
			default:
				s.telemetry.CapacityRejected("grpc")
				return nil, status.Error(codes.ResourceExhausted, "service capacity exhausted")
			}
		}
		response, err = next(ctx, req)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			err = status.FromContextError(err).Err()
		}
		return response, err
	}
}

// UnaryClientInterceptor propagates trace context without credentials or automatic retries.
func (s *RequestStack) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, conn *grpc.ClientConn, invoke grpc.UnaryInvoker, options ...grpc.CallOption) (err error) {
		ctx, observation := s.telemetry.StartRequest(ctx, "grpc-client", method, trace.SpanKindClient)
		defer func() {
			p := recover()
			outcome := grpcOutcome(err)
			if p != nil {
				outcome = "aborted"
			}
			observation.End(method, status.Code(err).String(), outcome)
			if p != nil {
				panic(p)
			}
		}()
		err = invoke(injectOutgoingTrace(ctx), method, req, reply, conn, options...)
		return err
	}
}
