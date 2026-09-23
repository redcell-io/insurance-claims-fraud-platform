// Command gateway runs ClaimsGateway: minimal REST ingress that forwards
// claim submissions to the Orchestration Service over gRPC.
//
// Walking-skeleton scope only — no auth, no rate limiting, no tenant
// resolution, hardcoded single tenant. See DESIGN.md §8 and
// build-order-plan for what's deferred to later.
package main

import (
	"log"
	"net/http"
	"os"

	"claimfraud/pkg/telemetry"
	"claimfraud/services/claims-gateway/internal/client"
	"claimfraud/services/claims-gateway/internal/handler"
)

func main() {
	httpAddr := getenv("GATEWAY_HTTP_ADDR", ":8080")
	orchestrationAddr := getenv("ORCHESTRATION_ADDR", "localhost:9091")
	tenantConfigAddr := getenv("TENANT_CONFIG_ADDR", "localhost:9094")

	logger := telemetry.NewLogger("claims-gateway")

	tenantConfig, err := client.DialTenantConfig(tenantConfigAddr, logger)
	if err != nil {
		log.Fatalf("dial tenant-config-svc at %s: %v", tenantConfigAddr, err)
	}
	defer tenantConfig.Close()

	orchestration, err := client.DialOrchestration(orchestrationAddr, logger)
	if err != nil {
		log.Fatalf("dial orchestration service at %s: %v", orchestrationAddr, err)
	}
	defer orchestration.Close()

	h := &handler.ClaimsHandler{TenantConfig: tenantConfig, Orchestration: orchestration, Logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/claims", h.SubmitClaim)
	mux.HandleFunc("/healthz", handler.Healthz)

	logger.Info("claims-gateway starting", "http_addr", httpAddr, "orchestration_addr", orchestrationAddr, "tenant_config_addr", tenantConfigAddr)
	if err := http.ListenAndServe(httpAddr, mux); err != nil {
		log.Fatalf("claims-gateway server failed: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
