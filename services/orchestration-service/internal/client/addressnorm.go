// Package client holds outbound gRPC clients used by the Orchestration
// Service to call enrichment/model services.
package client

import (
	"context"
	"log/slog"
	"time"

	"claimfraud/pkg/telemetry"
	addressnormv1 "claimfraud/proto/gen/go/addressnorm/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// AddressNormClient calls the (Java, Spring Boot) Address Normalization
// service.
type AddressNormClient struct {
	conn *grpc.ClientConn
	rpc  addressnormv1.AddressNormalizationServiceClient
}

// DialAddressNorm connects at addr. x-correlation-id metadata is added
// automatically by telemetry.UnaryClientInterceptor whenever the call's
// ctx carries one (see telemetry.WithCorrelationID) — callers no longer
// need to append it by hand.
func DialAddressNorm(addr string, logger *slog.Logger) (*AddressNormClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(telemetry.UnaryClientInterceptor(logger)),
	)
	if err != nil {
		return nil, err
	}
	telemetry.WarmUp(conn, logger, addr, 5*time.Second)
	return &AddressNormClient{conn: conn, rpc: addressnormv1.NewAddressNormalizationServiceClient(conn)}, nil
}

func (c *AddressNormClient) Close() error {
	return c.conn.Close()
}

func (c *AddressNormClient) Normalize(ctx context.Context, correlationID, rawAddress string) (*addressnormv1.NormalizeResponse, error) {
	return c.rpc.Normalize(ctx, &addressnormv1.NormalizeRequest{
		CorrelationId: correlationID,
		RawAddress:    rawAddress,
	})
}
