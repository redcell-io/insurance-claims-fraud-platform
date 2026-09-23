// Package client holds outbound gRPC clients used by the Orchestration
// Service to call enrichment/model services and the Tenant Config Service.
package client

import (
	"context"
	"log/slog"
	"time"

	"claimfraud/pkg/telemetry"
	tenantconfigv1 "claimfraud/proto/gen/go/tenantconfig/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TenantConfigClient wraps the generated gRPC client for the Tenant Config
// Service.
type TenantConfigClient struct {
	conn *grpc.ClientConn
	rpc  tenantconfigv1.TenantConfigServiceClient
}

func DialTenantConfig(addr string, logger *slog.Logger) (*TenantConfigClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(telemetry.UnaryClientInterceptor(logger)),
	)
	if err != nil {
		return nil, err
	}
	telemetry.WarmUp(conn, logger, addr, 5*time.Second)
	return &TenantConfigClient{conn: conn, rpc: tenantconfigv1.NewTenantConfigServiceClient(conn)}, nil
}

func (c *TenantConfigClient) Close() error {
	return c.conn.Close()
}

// GetDagVersion resolves which DAG config version to load for
// (tenantID, product) — DESIGN.md §6.1 / §7's `dag_config_ref`.
func (c *TenantConfigClient) GetDagVersion(ctx context.Context, tenantID, product string) (int32, error) {
	resp, err := c.rpc.GetDagVersion(ctx, &tenantconfigv1.GetDagVersionRequest{
		TenantId: tenantID,
		Product:  product,
	})
	if err != nil {
		return 0, err
	}
	return resp.GetVersion(), nil
}
