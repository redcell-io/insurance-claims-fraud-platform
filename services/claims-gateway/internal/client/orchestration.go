// Package client holds outbound gRPC clients used by ClaimsGateway.
package client

import (
	"context"

	orchestrationv1 "claimfraud/proto/gen/go/orchestration/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
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
// (host:port).
func DialOrchestration(addr string) (*OrchestrationClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &OrchestrationClient{
		conn: conn,
		rpc:  orchestrationv1.NewOrchestrationServiceClient(conn),
	}, nil
}

func (c *OrchestrationClient) Close() error {
	return c.conn.Close()
}

// ProcessClaim calls the Orchestration Service, injecting tenantID and
// correlationID as gRPC metadata rather than the message payload — every
// downstream service reads tenant context via interceptor, per
// DESIGN.md §8.
func (c *OrchestrationClient) ProcessClaim(
	ctx context.Context,
	tenantID, correlationID string,
	req *orchestrationv1.ProcessClaimRequest,
) (*orchestrationv1.ProcessClaimResponse, error) {
	md := metadata.Pairs(
		"x-tenant-id", tenantID,
		"x-correlation-id", correlationID,
	)
	ctx = metadata.NewOutgoingContext(ctx, md)
	return c.rpc.ProcessClaim(ctx, req)
}
