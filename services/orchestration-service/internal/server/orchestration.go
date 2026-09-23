// Package server implements the OrchestrationService gRPC server.
package server

import (
	"context"

	"claimfraud/pkg/telemetry"
	orchestrationv1 "claimfraud/proto/gen/go/orchestration/v1"
	"claimfraud/services/orchestration-service/internal/dag"
)

// Server implements orchestrationv1.OrchestrationServiceServer.
type Server struct {
	orchestrationv1.UnimplementedOrchestrationServiceServer
	Executor *dag.Executor
}

// ProcessClaim reads tenant_id/correlation_id off ctx — populated by
// telemetry.UnaryServerInterceptor from incoming x-tenant-id/
// x-correlation-id metadata (DESIGN.md §8), the real interceptor this
// used to hand-roll via a local firstMetadataValue helper.
func (s *Server) ProcessClaim(ctx context.Context, req *orchestrationv1.ProcessClaimRequest) (*orchestrationv1.ProcessClaimResponse, error) {
	tenantID := telemetry.TenantIDFrom(ctx)
	correlationID := req.GetClaim().GetCorrelationId()
	if correlationID == "" {
		correlationID = telemetry.CorrelationIDFrom(ctx)
	}

	result, err := s.Executor.Run(ctx, tenantID, correlationID, req.GetClaim())
	if err != nil {
		return nil, err
	}

	return &orchestrationv1.ProcessClaimResponse{
		ClaimId:           req.GetClaim().GetClaimId(),
		CorrelationId:     correlationID,
		Status:            result.Status,
		NormalizedAddress: result.NormalizedAddress,
		FraudScore:        result.FraudScore,
		ModelVersion:      result.ModelVersion,
	}, nil
}
