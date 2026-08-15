// Package client holds outbound gRPC clients used by the Orchestration
// Service to call enrichment/model services.
package client

import (
	"context"

	addressnormv1 "claimfraud/proto/gen/go/addressnorm/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// AddressNormClient calls the (Java, Spring Boot) Address Normalization
// service.
type AddressNormClient struct {
	conn *grpc.ClientConn
	rpc  addressnormv1.AddressNormalizationServiceClient
}

func DialAddressNorm(addr string) (*AddressNormClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &AddressNormClient{conn: conn, rpc: addressnormv1.NewAddressNormalizationServiceClient(conn)}, nil
}

func (c *AddressNormClient) Close() error {
	return c.conn.Close()
}

func (c *AddressNormClient) Normalize(ctx context.Context, correlationID, rawAddress string) (*addressnormv1.NormalizeResponse, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "x-correlation-id", correlationID)
	return c.rpc.Normalize(ctx, &addressnormv1.NormalizeRequest{
		CorrelationId: correlationID,
		RawAddress:    rawAddress,
	})
}
