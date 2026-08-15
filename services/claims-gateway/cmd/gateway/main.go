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

	"claimfraud/services/claims-gateway/internal/client"
	"claimfraud/services/claims-gateway/internal/handler"
)

func main() {
	httpAddr := getenv("GATEWAY_HTTP_ADDR", ":8080")
	orchestrationAddr := getenv("ORCHESTRATION_ADDR", "localhost:9091")

	orchestration, err := client.DialOrchestration(orchestrationAddr)
	if err != nil {
		log.Fatalf("dial orchestration service at %s: %v", orchestrationAddr, err)
	}
	defer orchestration.Close()

	h := &handler.ClaimsHandler{Orchestration: orchestration}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/claims", h.SubmitClaim)
	mux.HandleFunc("/healthz", handler.Healthz)

	log.Printf("claims-gateway listening on %s, forwarding to orchestration at %s", httpAddr, orchestrationAddr)
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
