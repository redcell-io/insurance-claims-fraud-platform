package telemetry

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	metadataTenantID      = "x-tenant-id"
	metadataCorrelationID = "x-correlation-id"
)

// UnaryServerInterceptor extracts x-tenant-id/x-correlation-id from
// incoming gRPC metadata (if present), stores them on the context so
// every downstream call in the handler can read them via TenantIDFrom/
// CorrelationIDFrom (and so FromContext(ctx, logger) picks them up
// automatically), then logs one structured line per request.
//
// This replaces the hand-rolled firstMetadataValue helper that used to
// live in orchestration-service's own server package — every Go service
// now goes through the same extraction/logging path instead of
// reimplementing it per service.
func UnaryServerInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if v := firstValue(md, metadataTenantID); v != "" {
				ctx = WithTenantID(ctx, v)
			}
			if v := firstValue(md, metadataCorrelationID); v != "" {
				ctx = WithCorrelationID(ctx, v)
			}
		}

		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		l := FromContext(ctx, logger).With(
			"method", info.FullMethod,
			"duration_ms", duration.Milliseconds(),
		)
		if err != nil {
			l.Error("grpc request failed", "error", err)
		} else {
			l.Info("grpc request")
		}
		return resp, err
	}
}

// UnaryClientInterceptor injects x-tenant-id/x-correlation-id into
// outgoing gRPC metadata whenever ctx carries them (set upstream via
// WithTenantID/WithCorrelationID — typically once, at ClaimsGateway's
// ingress, then propagated automatically through every downstream call
// made with that context), then logs one structured line per call.
//
// This replaces every call site that used to hand-build
// metadata.Pairs("x-tenant-id", ..., "x-correlation-id", ...) itself.
func UnaryClientInterceptor(logger *slog.Logger) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		pairs := make([]string, 0, 4)
		if id := TenantIDFrom(ctx); id != "" {
			pairs = append(pairs, metadataTenantID, id)
		}
		if id := CorrelationIDFrom(ctx); id != "" {
			pairs = append(pairs, metadataCorrelationID, id)
		}
		if len(pairs) > 0 {
			ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
		}

		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		l := FromContext(ctx, logger).With(
			"method", method,
			"duration_ms", duration.Milliseconds(),
		)
		if err != nil {
			l.Error("grpc call failed", "error", err)
		} else {
			l.Info("grpc call")
		}
		return err
	}
}

func firstValue(md metadata.MD, key string) string {
	vals := md.Get(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}
