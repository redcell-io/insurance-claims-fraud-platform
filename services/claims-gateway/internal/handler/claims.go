// Package handler implements ClaimsGateway's REST surface.
package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	claimsv1 "claimfraud/proto/gen/go/claims/v1"
	orchestrationv1 "claimfraud/proto/gen/go/orchestration/v1"
	tenantconfigv1 "claimfraud/proto/gen/go/tenantconfig/v1"
	"claimfraud/services/claims-gateway/internal/client"
	"claimfraud/services/claims-gateway/internal/correlation"
)

// hardcodedTenantID stands in for real tenant resolution (API key/JWT →
// ResolveTenant, see DESIGN.md §8) until API-key auth exists. It's now used
// as a lookup key into a real Tenant Config Service call (see
// client.TenantConfigClient.GetTenant) rather than assuming an active
// tenant outright — proves the status-gating path even though "which
// tenant" is still not client-supplied. Rate limiting and scope checks are
// still skipped entirely — see build-order-plan.
const hardcodedTenantID = "acme_insurance"

const (
	orchestrationTimeout = 5 * time.Second
	tenantConfigTimeout  = 2 * time.Second
)

// ClaimsHandler handles the claim-submission REST endpoint and forwards
// to Orchestration over gRPC.
type ClaimsHandler struct {
	TenantConfig  *client.TenantConfigClient
	Orchestration *client.OrchestrationClient
}

// claimRequest mirrors the subset of claims.v1.ClaimEvent accepted over
// REST. tenant_id is intentionally absent — it's not a client-supplied
// field (see hardcodedTenantID above / DESIGN.md §8).
type claimRequest struct {
	ClaimID      string `json:"claim_id"`
	Product      string `json:"product"`
	EventType    string `json:"event_type"`
	PolicyNumber string `json:"policy_number"`
	ClaimantName string `json:"claimant_name"`
	RawAddress   string `json:"raw_address"`
}

type claimResponse struct {
	ClaimID           string  `json:"claim_id"`
	CorrelationID     string  `json:"correlation_id"`
	Status            string  `json:"status"`
	NormalizedAddress string  `json:"normalized_address"`
	FraudScore        float64 `json:"fraud_score"`
	ModelVersion      string  `json:"model_version"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (h *ClaimsHandler) SubmitClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req claimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.ClaimID == "" || req.Product == "" || req.EventType == "" {
		writeError(w, http.StatusBadRequest, "claim_id, product, and event_type are required")
		return
	}

	correlationID := correlation.New()

	// Tenant resolution + status check (DESIGN.md §8 steps 2-3), before
	// anything downstream runs. Real API-key resolution is still deferred
	// (see hardcodedTenantID above) but the status gate itself is real.
	tenantCtx, err := h.resolveTenant(r.Context(), correlationID)
	if err != nil {
		log.Printf("correlation_id=%s tenant resolution failed for %s: %v", correlationID, hardcodedTenantID, err)
		writeError(w, http.StatusForbidden, "tenant not found or unavailable")
		return
	}
	if tenantCtx.GetStatus() != "active" {
		log.Printf("correlation_id=%s tenant %s not active (status=%s)", correlationID, hardcodedTenantID, tenantCtx.GetStatus())
		writeError(w, http.StatusForbidden, "tenant is not active")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), orchestrationTimeout)
	defer cancel()

	resp, err := h.Orchestration.ProcessClaim(ctx, hardcodedTenantID, correlationID,
		&orchestrationv1.ProcessClaimRequest{
			Claim: &claimsv1.ClaimEvent{
				ClaimId:       req.ClaimID,
				CorrelationId: correlationID,
				Product:       req.Product,
				EventType:     req.EventType,
				PolicyNumber:  req.PolicyNumber,
				ClaimantName:  req.ClaimantName,
				RawAddress:    req.RawAddress,
			},
		})
	if err != nil {
		log.Printf("correlation_id=%s orchestration call failed: %v", correlationID, err)
		writeError(w, http.StatusBadGateway, "orchestration call failed")
		return
	}

	writeJSON(w, http.StatusOK, claimResponse{
		ClaimID:           resp.ClaimId,
		CorrelationID:     resp.CorrelationId,
		Status:            resp.Status,
		NormalizedAddress: resp.NormalizedAddress,
		FraudScore:        resp.FraudScore,
		ModelVersion:      resp.ModelVersion,
	})
}

// resolveTenant calls the Tenant Config Service for hardcodedTenantID.
func (h *ClaimsHandler) resolveTenant(parent context.Context, correlationID string) (*tenantconfigv1.TenantContext, error) {
	ctx, cancel := context.WithTimeout(parent, tenantConfigTimeout)
	defer cancel()
	return h.TenantConfig.GetTenant(ctx, hardcodedTenantID)
}

func Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
