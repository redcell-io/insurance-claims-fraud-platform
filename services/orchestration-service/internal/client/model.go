package client

import (
	"context"
	"log/slog"
	"time"

	"claimfraud/pkg/telemetry"
	modelv1 "claimfraud/proto/gen/go/model/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ModelClient calls the (stub) Model Service.
type ModelClient struct {
	conn *grpc.ClientConn
	rpc  modelv1.ModelServiceClient
}

func DialModel(addr string, logger *slog.Logger) (*ModelClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(telemetry.UnaryClientInterceptor(logger)),
	)
	if err != nil {
		return nil, err
	}
	telemetry.WarmUp(conn, logger, addr, 5*time.Second)
	return &ModelClient{conn: conn, rpc: modelv1.NewModelServiceClient(conn)}, nil
}

func (c *ModelClient) Close() error {
	return c.conn.Close()
}

func (c *ModelClient) Score(ctx context.Context, correlationID string, features map[string]string) (*modelv1.ScoreResponse, error) {
	return c.rpc.Score(ctx, &modelv1.ScoreRequest{
		CorrelationId: correlationID,
		Features:      features,
	})
}
