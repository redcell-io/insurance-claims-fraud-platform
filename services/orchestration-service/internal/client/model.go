package client

import (
	"context"

	modelv1 "claimfraud/proto/gen/go/model/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// ModelClient calls the (stub) Model Service.
type ModelClient struct {
	conn *grpc.ClientConn
	rpc  modelv1.ModelServiceClient
}

func DialModel(addr string) (*ModelClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &ModelClient{conn: conn, rpc: modelv1.NewModelServiceClient(conn)}, nil
}

func (c *ModelClient) Close() error {
	return c.conn.Close()
}

func (c *ModelClient) Score(ctx context.Context, correlationID string, features map[string]string) (*modelv1.ScoreResponse, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "x-correlation-id", correlationID)
	return c.rpc.Score(ctx, &modelv1.ScoreRequest{
		CorrelationId: correlationID,
		Features:      features,
	})
}
