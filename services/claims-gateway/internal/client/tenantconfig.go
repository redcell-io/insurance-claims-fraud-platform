// Package client holds outbound gRPC clients used by ClaimsGateway.
package client

import (
	"context"

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

// DialTenantConfig connects to the Tenant Config Service at addr (host:port).
func DialTenantConfig(addr string) (*TenantConfigClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &TenantConfigClient{
		conn: conn,
		rpc:  tenantconfigv1.NewTenantConfigServiceClient(conn),
	}, nil
}

func (c *TenantConfigClient) Close() error {
	return c.conn.Close()
}

// GetTenant resolves tenant status/display data. Stands in for the real
// ResolveTenant(api_key_hash) (DESIGN.md §7) until API-key auth exists —
// see the hardcodedTenantID comment in handler/claims.go.
func (c *TenantConfigClient) GetTenant(ctx context.Context, tenantID string) (*tenantconfigv1.TenantContext, error) {
	return c.rpc.GetTenant(ctx, &tenantconfigv1.GetTenantRequest{TenantId: tenantID})
}
