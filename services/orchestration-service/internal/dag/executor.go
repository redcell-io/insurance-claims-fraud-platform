package dag

import (
	"context"
	"fmt"
	"log"

	claimsv1 "claimfraud/proto/gen/go/claims/v1"

	addressnormv1 "claimfraud/proto/gen/go/addressnorm/v1"
	modelv1 "claimfraud/proto/gen/go/model/v1"
)

// AddressNormalizer and Scorer are the narrow interfaces the executor needs
// from its downstream clients, so it can be tested without a live gRPC
// connection.
type AddressNormalizer interface {
	Normalize(ctx context.Context, correlationID, rawAddress string) (*addressnormv1.NormalizeResponse, error)
}

type Scorer interface {
	Score(ctx context.Context, correlationID string, features map[string]string) (*modelv1.ScoreResponse, error)
}

// Result is what a DAG run produces for a claim.
type Result struct {
	Status            string // "scored" | "degraded"
	NormalizedAddress string
	FraudScore        float64
	ModelVersion      string
}

// ConfigLoader is the narrow interface Executor needs to fetch a DAG
// Config, so it can be tested with an in-memory Config instead of a real
// Loader (disk + tenant-config-svc round trip).
type ConfigLoader interface {
	Load(ctx context.Context, tenantID, product, eventType string) (*Config, error)
}

// Executor runs a Config-driven DAG for a claim.
//
// This is a *staged* executor, not a fully general topological-sort DAG
// engine: it recognizes exactly the three stages DESIGN.md §6.1's example
// DAG has — an "enrichment" group (nodes with no depends_on, run
// concurrently... today sequentially, since there's only one), then nodes
// depending on that group ("model_score"), then nodes depending on
// model_score ("publish"). That's sufficient for every DAG this repo has
// today, including layer 2b's planned claimant_id_hash/policy_lookup (they
// join the enrichment group; they don't depend on each other). A real
// topo-sort is deferred until a DAG actually needs multi-level dependencies
// beyond this shape.
type Executor struct {
	Loader      ConfigLoader
	AddressNorm AddressNormalizer
	Model       Scorer
}

const groupEnrichment = "enrichment"
const nodeModelScore = "model_score"

func (e *Executor) Run(ctx context.Context, tenantID, correlationID string, claim *claimsv1.ClaimEvent) (*Result, error) {
	cfg, err := e.Loader.Load(ctx, tenantID, claim.GetProduct(), claim.GetEventType())
	if err != nil {
		return nil, fmt.Errorf("load dag config: %w", err)
	}

	status := "scored"
	normalizedAddress := claim.GetRawAddress()

	// Stage 1: enrichment group — nodes with no depends_on.
	for _, node := range cfg.Nodes {
		if len(node.DependsOn) != 0 || !node.EnabledForTenant(tenantID) {
			continue
		}
		switch node.Service {
		case "address-normalization-svc":
			normResp, err := e.AddressNorm.Normalize(ctx, correlationID, claim.GetRawAddress())
			if err != nil {
				if !nodeFailureHandled(node, correlationID, err) {
					return nil, err
				}
				status = "degraded"
				continue
			}
			normalizedAddress = normResp.GetNormalizedAddress()
		default:
			log.Printf("correlation_id=%s dag node=%s: unrecognized service %q, skipping", correlationID, node.ID, node.Service)
		}
	}

	// Stage 2: nodes depending on the enrichment group (model_score).
	var scoreResp *modelv1.ScoreResponse
	for _, node := range cfg.Nodes {
		if !dependsOn(node, groupEnrichment) || !node.EnabledForTenant(tenantID) {
			continue
		}
		switch node.Service {
		case "model-service":
			features := map[string]string{
				"tenant_id":          tenantID,
				"product":            claim.GetProduct(),
				"event_type":         claim.GetEventType(),
				"policy_number":      claim.GetPolicyNumber(),
				"claimant_name":      claim.GetClaimantName(),
				"normalized_address": normalizedAddress,
			}
			resp, err := e.Model.Score(ctx, correlationID, features)
			if err != nil {
				if !nodeFailureHandled(node, correlationID, err) {
					return nil, err
				}
				continue
			}
			scoreResp = resp
		default:
			log.Printf("correlation_id=%s dag node=%s: unrecognized service %q, skipping", correlationID, node.ID, node.Service)
		}
	}
	if scoreResp == nil {
		return nil, fmt.Errorf("dag: no model_score result produced")
	}

	// Stage 3: nodes depending on model_score (publish) — stubbed, logs
	// only. Real version: kafka-publisher to claims.realtime (DESIGN.md §10).
	for _, node := range cfg.Nodes {
		if !dependsOn(node, nodeModelScore) || !node.EnabledForTenant(tenantID) {
			continue
		}
		switch node.Service {
		case "kafka-publisher":
			log.Printf(
				"correlation_id=%s tenant_id=%s claim_id=%s STUB-PUBLISH status=%s fraud_score=%.4f model_version=%s",
				correlationID, tenantID, claim.GetClaimId(), status, scoreResp.GetScore(), scoreResp.GetModelVersion(),
			)
		default:
			log.Printf("correlation_id=%s dag node=%s: unrecognized service %q, skipping", correlationID, node.ID, node.Service)
		}
	}

	return &Result{
		Status:            status,
		NormalizedAddress: normalizedAddress,
		FraudScore:        scoreResp.GetScore(),
		ModelVersion:      scoreResp.GetModelVersion(),
	}, nil
}

func dependsOn(node Node, name string) bool {
	for _, d := range node.DependsOn {
		if d == name {
			return true
		}
	}
	return false
}

// nodeFailureHandled logs a node failure per its on_failure policy and
// reports whether the executor should continue (skip/degrade) rather than
// abort the whole run (fail_fast). skip and degrade are both treated as
// "continue, best-effort" here — DESIGN.md §6.1 distinguishes them, but
// this repo doesn't yet have a node where the difference in downstream
// behavior matters, so both simply mark the overall result "degraded".
func nodeFailureHandled(node Node, correlationID string, err error) bool {
	policy := node.EffectiveOnFailure()
	switch policy {
	case OnFailureSkip, OnFailureDegrade:
		log.Printf("correlation_id=%s dag node=%s failed, continuing (on_failure=%s): %v", correlationID, node.ID, policy, err)
		return true
	default: // fail_fast
		log.Printf("correlation_id=%s dag node=%s failed (on_failure=fail_fast): %v", correlationID, node.ID, err)
		return false
	}
}
