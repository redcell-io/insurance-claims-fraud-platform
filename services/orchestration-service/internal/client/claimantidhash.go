package client

import (
	"context"

	claimantidhashv1 "claimfraud/proto/gen/go/claimantidhash/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// ClaimantIDHashClient calls the (Java, Spring Boot) Claimant ID Hashing
// service.
type ClaimantIDHashClient struct {
	conn *grpc.ClientConn
	rpc  claimantidhashv1.ClaimantIdHashingServiceClient
}

func DialClaimantIDHash(addr string) (*ClaimantIDHashClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &ClaimantIDHashClient{conn: conn, rpc: claimantidhashv1.NewClaimantIdHashingServiceClient(conn)}, nil
}

func (c *ClaimantIDHashClient) Close() error {
	return c.conn.Close()
}

func (c *ClaimantIDHashClient) Hash(ctx context.Context, correlationID, claimantName string) (*claimantidhashv1.HashClaimantIdResponse, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "x-correlation-id", correlationID)
	return c.rpc.HashClaimantId(ctx, &claimantidhashv1.HashClaimantIdRequest{
		CorrelationId: correlationID,
		ClaimantName:  claimantName,
	})
}
