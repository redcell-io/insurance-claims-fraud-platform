// Package dag executes the enrichment DAG for a claim.
//
// This is a HARDCODED single-path DAG, not the config-driven engine
// described in DESIGN.md §6.1 (tenant/product/eventType-keyed, YAML,
// hot-reloadable via Kafka). It exists to prove the orchestration→
// enrichment call pattern end-to-end before that engine is built — see
// build-order-plan. The node sequence mirrors the example DAG in
// DESIGN.md §6.1, trimmed to what the walking skeleton has real services
// for:
//
//  1. address_normalize (group: enrichment, on_failure: skip)
//  2. model_score        (depends_on: [enrichment])
//  3. publish             (depends_on: [model_score], stubbed — logs only)
package dag

import (
	"context"
	"log"

	claimsv1 "claimfraud/proto/gen/go/claims/v1"

	addressnormv1 "claimfraud/proto/gen/go/addressnorm/v1"
	modelv1 "claimfraud/proto/gen/go/model/v1"
)

// AddressNormalizer and Scorer are the narrow interfaces the DAG needs
// from its downstream clients, so the executor can be tested without a
// live gRPC connection.
type AddressNormalizer interface {
	Normalize(ctx context.Context, correlationID, rawAddress string) (*addressnormv1.NormalizeResponse, error)
}

type Scorer interface {
	Score(ctx context.Context, correlationID string, features map[string]string) (*modelv1.ScoreResponse, error)
}

// Result is what the hardcoded DAG produces for a claim.
type Result struct {
	Status             string // "scored" | "degraded"
	NormalizedAddress  string
	FraudScore         float64
	ModelVersion       string
}

// Executor runs the hardcoded node sequence.
type Executor struct {
	AddressNorm AddressNormalizer
	Model       Scorer
}

func (e *Executor) Run(ctx context.Context, tenantID, correlationID string, claim *claimsv1.ClaimEvent) (*Result, error) {
	status := "scored"

	// Node: address_normalize (group: enrichment, on_failure: skip)
	normalizedAddress := claim.GetRawAddress()
	normResp, err := e.AddressNorm.Normalize(ctx, correlationID, claim.GetRawAddress())
	if err != nil {
		// on_failure: skip — degrade gracefully rather than fail_fast,
		// per the address_normalize node policy in DESIGN.md §6.1.
		log.Printf("correlation_id=%s address_normalize failed, skipping (degraded): %v", correlationID, err)
		status = "degraded"
	} else {
		normalizedAddress = normResp.GetNormalizedAddress()
	}

	// Node: model_score (depends_on: [enrichment])
	features := map[string]string{
		"tenant_id":          tenantID,
		"product":            claim.GetProduct(),
		"event_type":         claim.GetEventType(),
		"policy_number":      claim.GetPolicyNumber(),
		"claimant_name":      claim.GetClaimantName(),
		"normalized_address": normalizedAddress,
	}
	scoreResp, err := e.Model.Score(ctx, correlationID, features)
	if err != nil {
		// model_score has no on_failure override in the example DAG,
		// i.e. it takes the DAG-level default — treated as fail_fast
		// here since a score is the point of the request.
		return nil, err
	}

	// Node: publish (depends_on: [model_score]) — stubbed, logs only.
	// Real version: kafka-publisher to claims.realtime (DESIGN.md §10).
	log.Printf(
		"correlation_id=%s tenant_id=%s claim_id=%s STUB-PUBLISH status=%s fraud_score=%.4f model_version=%s",
		correlationID, tenantID, claim.GetClaimId(), status, scoreResp.GetScore(), scoreResp.GetModelVersion(),
	)

	return &Result{
		Status:            status,
		NormalizedAddress: normalizedAddress,
		FraudScore:        scoreResp.GetScore(),
		ModelVersion:      scoreResp.GetModelVersion(),
	}, nil
}
