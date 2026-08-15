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
	"claimfraud/services/claims-gateway/internal/client"
	"claimfraud/services/claims-gateway/internal/correlation"
)

// hardcodedTenantID stands in for real tenant resolution (API key/JWT →
// ResolveTenant, see DESIGN.md §8) until the Tenant Config Service exists.
// Auth, rate limiting, and scope checks are likewise skipped for the
// walking skeleton — see build-order-plan.
const hardcodedTenantID = "acme_insurance"

const orchestrationTimeout = 5 * time.Second

// ClaimsHandler handles the claim-submission REST endpoint and forwards
// to Orchestration over gRPC.
type ClaimsHandler struct {
	Orchestration *client.OrchestrationClient
}

// claimRequest mirrors the subset of claims.v1.ClaimEvent accepted over
// REST. tenant_id is intentionally absent — it's not a client-supplied
// field (see hardcodedTenantID above / DESIGN.md §8).
type claimRequest struct {
	ClaimID       string `json:"claim_id"`
	Product       string `json:"product"`
	EventType     string `json:"event_type"`
	PolicyNumber  string `json:"policy_number"`
	ClaimantName  string `json:"claimant_name"`
	RawAddress    string `json:"raw_address"`
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
