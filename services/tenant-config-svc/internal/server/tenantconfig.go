// Package server implements the TenantConfigService gRPC server.
package server

import (
	"context"

	tenantconfigv1 "claimfraud/proto/gen/go/tenantconfig/v1"
	"claimfraud/services/tenant-config-svc/internal/store"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements tenantconfigv1.TenantConfigServiceServer, backed by an
// in-memory Store loaded at startup (see internal/store).
type Server struct {
	tenantconfigv1.UnimplementedTenantConfigServiceServer
	Store *store.Store
}

func (s *Server) GetTenant(ctx context.Context, req *tenantconfigv1.GetTenantRequest) (*tenantconfigv1.TenantContext, error) {
	t, ok := s.Store.Get(req.GetTenantId())
	if !ok {
		return nil, status.Errorf(codes.NotFound, "tenant %q not found", req.GetTenantId())
	}
	return &tenantconfigv1.TenantContext{
		TenantId:    t.TenantID,
		DisplayName: t.DisplayName,
		Status:      t.Status,
	}, nil
}

func (s *Server) GetDagVersion(ctx context.Context, req *tenantconfigv1.GetDagVersionRequest) (*tenantconfigv1.GetDagVersionResponse, error) {
	v, ok := s.Store.DagVersion(req.GetTenantId(), req.GetProduct())
	if !ok {
		return nil, status.Errorf(codes.NotFound, "no dag version configured for tenant %q product %q", req.GetTenantId(), req.GetProduct())
	}
	return &tenantconfigv1.GetDagVersionResponse{Version: v}, nil
}
