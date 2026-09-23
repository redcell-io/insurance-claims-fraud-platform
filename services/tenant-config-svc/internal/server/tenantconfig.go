// Package server implements the TenantConfigService gRPC server.
package server

import (
	"context"
	"log/slog"

	"claimfraud/pkg/telemetry"
	tenantconfigv1 "claimfraud/proto/gen/go/tenantconfig/v1"
	"claimfraud/services/tenant-config-svc/internal/store"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements tenantconfigv1.TenantConfigServiceServer, backed by an
// in-memory Store loaded at startup (see internal/store).
//
// Previously had zero logging on either RPC and never received a
// correlation ID at all — closed by telemetry.UnaryServerInterceptor
// (wired in cmd/tenantconfig/main.go), which now extracts
// x-correlation-id from incoming metadata the same way every other Go
// service does, plus the explicit logging below.
type Server struct {
	tenantconfigv1.UnimplementedTenantConfigServiceServer
	Store  *store.Store
	Logger *slog.Logger
}

func (s *Server) GetTenant(ctx context.Context, req *tenantconfigv1.GetTenantRequest) (*tenantconfigv1.TenantContext, error) {
	logger := telemetry.FromContext(ctx, s.Logger)
	t, ok := s.Store.Get(req.GetTenantId())
	if !ok {
		logger.Warn("get tenant: not found", "tenant_id", req.GetTenantId())
		return nil, status.Errorf(codes.NotFound, "tenant %q not found", req.GetTenantId())
	}
	logger.Info("get tenant", "tenant_id", t.TenantID, "status", t.Status)
	return &tenantconfigv1.TenantContext{
		TenantId:    t.TenantID,
		DisplayName: t.DisplayName,
		Status:      t.Status,
	}, nil
}

func (s *Server) GetDagVersion(ctx context.Context, req *tenantconfigv1.GetDagVersionRequest) (*tenantconfigv1.GetDagVersionResponse, error) {
	// Doesn't re-add tenant_id here (unlike GetTenant below) — by the time
	// this is reached via the normal request chain, ctx already carries it
	// (from Executor.Run, via the client interceptor's outgoing metadata),
	// so FromContext already supplies it; re-adding it would just duplicate
	// the key. Only missing if this RPC is invoked directly without that
	// metadata (e.g. a raw console test) — product/version still identify
	// the call either way.
	logger := telemetry.FromContext(ctx, s.Logger)
	v, ok := s.Store.DagVersion(req.GetTenantId(), req.GetProduct())
	if !ok {
		logger.Warn("get dag version: not found", "product", req.GetProduct())
		return nil, status.Errorf(codes.NotFound, "no dag version configured for tenant %q product %q", req.GetTenantId(), req.GetProduct())
	}
	logger.Info("get dag version", "product", req.GetProduct(), "version", v)
	return &tenantconfigv1.GetDagVersionResponse{Version: v}, nil
}
