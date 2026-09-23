package client

import (
	"context"
	"log/slog"
	"time"

	"claimfraud/pkg/telemetry"
	policylookupv1 "claimfraud/proto/gen/go/policylookup/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// PolicyLookupClient calls the (Java, Spring Boot) Policy Lookup service.
type PolicyLookupClient struct {
	conn *grpc.ClientConn
	rpc  policylookupv1.PolicyLookupServiceClient
}

func DialPolicyLookup(addr string, logger *slog.Logger) (*PolicyLookupClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(telemetry.UnaryClientInterceptor(logger)),
	)
	if err != nil {
		return nil, err
	}
	telemetry.WarmUp(conn, logger, addr, 5*time.Second)
	return &PolicyLookupClient{conn: conn, rpc: policylookupv1.NewPolicyLookupServiceClient(conn)}, nil
}

func (c *PolicyLookupClient) Close() error {
	return c.conn.Close()
}

func (c *PolicyLookupClient) Lookup(ctx context.Context, correlationID, policyNumber string) (*policylookupv1.LookupPolicyResponse, error) {
	return c.rpc.LookupPolicy(ctx, &policylookupv1.LookupPolicyRequest{
		CorrelationId: correlationID,
		PolicyNumber:  policyNumber,
	})
}
