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
	"net/http"
	"os"

	"claimfraud/pkg/telemetry"
	tenantconfigv1 "claimfraud/proto/gen/go/tenantconfig/v1"
	"claimfraud/services/tenant-config-svc/internal/server"
	"claimfraud/services/tenant-config-svc/internal/store"

	"google.golang.org/grpc"
)

func main() {
	grpcAddr := getenv("TENANT_CONFIG_GRPC_ADDR", ":9094")
	metricsAddr := getenv("TENANT_CONFIG_METRICS_ADDR", ":9194")
	tenantsFile := getenv("TENANTS_FILE", "../../config/tenants.yaml")

	logger := telemetry.NewLogger("tenant-config-svc")

	st, err := store.Load(tenantsFile)
	if err != nil {
		log.Fatalf("load tenants file %s: %v", tenantsFile, err)
	}

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("listen on %s: %v", grpcAddr, err)
	}

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(telemetry.UnaryServerInterceptor(logger)))
	tenantconfigv1.RegisterTenantConfigServiceServer(grpcServer, &server.Server{Store: st, Logger: logger})

	// No embedded web server here (gRPC only) — /metrics (DECISIONS.md
	// #18) gets its own small listener.
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", telemetry.MetricsHandler())
		if err := http.ListenAndServe(metricsAddr, mux); err != nil {
			logger.Error("metrics server failed", "error", err)
		}
	}()

	logger.Info("tenant-config-svc starting", "grpc_addr", grpcAddr, "metrics_addr", metricsAddr, "tenants_file", tenantsFile)
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
