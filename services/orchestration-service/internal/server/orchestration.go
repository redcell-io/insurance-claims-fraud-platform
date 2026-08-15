// Package server implements the OrchestrationService gRPC server.
package server

import (
	"context"

	orchestrationv1 "claimfraud/proto/gen/go/orchestration/v1"
	"claimfraud/services/orchestration-service/internal/dag"

	"google.golang.org/grpc/metadata"
)

// Server implements orchestrationv1.OrchestrationServiceServer.
type Server struct {
	orchestrationv1.UnimplementedOrchestrationServiceServer
	Executor *dag.Executor
}

func (s *Server) ProcessClaim(ctx context.Context, req *orchestrationv1.ProcessClaimRequest) (*orchestrationv1.ProcessClaimResponse, error) {
	tenantID := firstMetadataValue(ctx, "x-tenant-id")
	correlationID := req.GetClaim().GetCorrelationId()
	if correlationID == "" {
		correlationID = firstMetadataValue(ctx, "x-correlation-id")
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

// firstMetadataValue reads a value out of incoming gRPC metadata. This
// stands in for the real tenant-context interceptor described in
// DESIGN.md §8 — good enough for the walking skeleton, where the DAG
// itself is hardcoded regardless of tenant.
func firstMetadataValue(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}
