// Command tenantconfig runs the Tenant Config Service: a gRPC server
// serving tenant status and DAG version data from a local YAML file.
//
// Thin-slice scope only — see internal/store for what's deferred
// (Postgres, transactional outbox, Kafka cache invalidation). See
// DESIGN.md §7 for the full design.
package main

import (
	"log"
	"net"
	"os"

	tenantconfigv1 "claimfraud/proto/gen/go/tenantconfig/v1"
	"claimfraud/services/tenant-config-svc/internal/server"
	"claimfraud/services/tenant-config-svc/internal/store"

	"google.golang.org/grpc"
)

func main() {
	grpcAddr := getenv("TENANT_CONFIG_GRPC_ADDR", ":9094")
	tenantsFile := getenv("TENANTS_FILE", "../../config/tenants.yaml")

	st, err := store.Load(tenantsFile)
	if err != nil {
		log.Fatalf("load tenants file %s: %v", tenantsFile, err)
	}

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("listen on %s: %v", grpcAddr, err)
	}

	grpcServer := grpc.NewServer()
	tenantconfigv1.RegisterTenantConfigServiceServer(grpcServer, &server.Server{Store: st})

	log.Printf("tenant-config-svc listening on %s (tenants file=%s)", grpcAddr, tenantsFile)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("tenant-config-svc server failed: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
