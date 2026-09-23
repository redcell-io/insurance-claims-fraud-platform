// Package client holds outbound gRPC clients used by ClaimsGateway.
package client

import (
	"context"
	"log/slog"
	"time"

	"claimfraud/pkg/telemetry"
	orchestrationv1 "claimfraud/proto/gen/go/orchestration/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// OrchestrationClient wraps the generated gRPC client for the
// Orchestration Service. mTLS (see DESIGN.md §12) is not wired up yet —
// this dials with insecure transport credentials, fine for local dev on
// the walking skeleton, not for anything resembling production.
type OrchestrationClient struct {
	conn *grpc.ClientConn
	rpc  orchestrationv1.OrchestrationServiceClient
}

// DialOrchestration connects to the Orchestration Service at addr
// (host:port). Every call made through the returned client carries
// tenant_id/correlation_id as gRPC metadata automatically, via
// telemetry.UnaryClientInterceptor reading them off the call's context
// (see telemetry.WithTenantID/WithCorrelationID) — never the message
// payload, per DESIGN.md §8.
func DialOrchestration(addr string, logger *slog.Logger) (*OrchestrationClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(telemetry.UnaryClientInterceptor(logger)),
	)
	if err != nil {
		return nil, err
	}
	telemetry.WarmUp(conn, logger, addr, 5*time.Second)
	return &OrchestrationClient{
		conn: conn,
		rpc:  orchestrationv1.NewOrchestrationServiceClient(conn),
	}, nil
}

func (c *OrchestrationClient) Close() error {
	return c.conn.Close()
}

// ProcessClaim calls the Orchestration Service. ctx must carry
// tenant_id/correlation_id (see DialOrchestration) for them to reach the
// server as metadata.
func (c *OrchestrationClient) ProcessClaim(
	ctx context.Context,
	req *orchestrationv1.ProcessClaimRequest,
) (*orchestrationv1.ProcessClaimResponse, error) {
	return c.rpc.ProcessClaim(ctx, req)
}
