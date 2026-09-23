package dag

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	claimsv1 "claimfraud/proto/gen/go/claims/v1"

	"claimfraud/pkg/telemetry"
	addressnormv1 "claimfraud/proto/gen/go/addressnorm/v1"
	claimantidhashv1 "claimfraud/proto/gen/go/claimantidhash/v1"
	modelv1 "claimfraud/proto/gen/go/model/v1"
	policylookupv1 "claimfraud/proto/gen/go/policylookup/v1"
	"claimfraud/services/orchestration-service/internal/kafka"
)

// AddressNormalizer, ClaimantIDHasher, PolicyLookuper, and Scorer are the
// narrow interfaces the executor needs from its downstream clients, so it
// can be tested without a live gRPC connection.
type AddressNormalizer interface {
	Normalize(ctx context.Context, correlationID, rawAddress string) (*addressnormv1.NormalizeResponse, error)
}

type ClaimantIDHasher interface {
	Hash(ctx context.Context, correlationID, claimantName string) (*claimantidhashv1.HashClaimantIdResponse, error)
}

type PolicyLookuper interface {
	Lookup(ctx context.Context, correlationID, policyNumber string) (*policylookupv1.LookupPolicyResponse, error)
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
// today: claimant_id_hash/address_normalize/policy_lookup all join the
// enrichment group; none depend on each other. A real topo-sort is
// deferred until a DAG actually needs multi-level dependencies beyond this
// shape.
type Executor struct {
	Loader         ConfigLoader
	AddressNorm    AddressNormalizer
	ClaimantIDHash ClaimantIDHasher
	PolicyLookup   PolicyLookuper
	Model          Scorer
	Publisher      kafka.Publisher
	Logger         *slog.Logger
}

const groupEnrichment = "enrichment"
const nodeModelScore = "model_score"

// defaultNodeTimeout bounds a downstream call when neither the node's own
// timeout_ms nor its DAG's timeout_ms is set, so a config that omits both
// still can't hang forever — "trained-in defaults, not nulls" (DESIGN.md
// §9) applied to timeouts, not just feature values.
const defaultNodeTimeout = 500 * time.Millisecond

// retryBackoff is the fixed delay between retry attempts. Kept simple —
// no exponential backoff/jitter — proportionate to this pass's scope; see
// DECISIONS.md's entry for this layer.
const retryBackoff = 20 * time.Millisecond

func (e *Executor) Run(ctx context.Context, tenantID, correlationID string, claim *claimsv1.ClaimEvent) (*Result, error) {
	ctx = telemetry.WithCorrelationID(ctx, correlationID)
	ctx = telemetry.WithTenantID(ctx, tenantID)
	logger := telemetry.FromContext(ctx, e.Logger)

	cfg, err := e.Loader.Load(ctx, tenantID, claim.GetProduct(), claim.GetEventType())
	if err != nil {
		return nil, fmt.Errorf("load dag config: %w", err)
	}

	status := "scored"
	normalizedAddress := claim.GetRawAddress()
	claimantIDHash := ""
	policyStatus := ""
	policyCoverageType := ""
	// Defaults to false (not "unknown") on skip/degrade/never-ran — a
	// claim this service can't confidently verify the address for should
	// read as unverified to Model Service, not silently absent (DESIGN.md
	// §9: "trained-in defaults, not nulls"). See DECISIONS.md #16.
	addressValid := false

	// Stage 1: enrichment group — nodes with no depends_on.
	for _, node := range cfg.Nodes {
		if len(node.DependsOn) != 0 || !node.EnabledForTenant(tenantID) {
			continue
		}
		switch node.Service {
		case "claimant-id-hashing-svc":
			hashResp, err := callWithResilience(ctx, cfg, node, func(ctx context.Context) (*claimantidhashv1.HashClaimantIdResponse, error) {
				return e.ClaimantIDHash.Hash(ctx, correlationID, claim.GetClaimantName())
			})
			if err != nil {
				if !nodeFailureHandled(logger, node, err) {
					return nil, err
				}
				status = "degraded"
				continue
			}
			claimantIDHash = hashResp.GetClaimantIdHash()
		case "address-normalization-svc":
			normResp, err := callWithResilience(ctx, cfg, node, func(ctx context.Context) (*addressnormv1.NormalizeResponse, error) {
				return e.AddressNorm.Normalize(ctx, correlationID, claim.GetRawAddress())
			})
			if err != nil {
				if !nodeFailureHandled(logger, node, err) {
					return nil, err
				}
				status = "degraded"
				continue
			}
			normalizedAddress = normResp.GetNormalizedAddress()
			addressValid = normResp.GetValid()
		case "policy-lookup-svc":
			// found=false is a normal business result (unknown policy
			// number), not a call failure — on_failure only applies to the
			// RPC itself erroring out, so this doesn't go through
			// nodeFailureHandled.
			lookupResp, err := callWithResilience(ctx, cfg, node, func(ctx context.Context) (*policylookupv1.LookupPolicyResponse, error) {
				return e.PolicyLookup.Lookup(ctx, correlationID, claim.GetPolicyNumber())
			})
			if err != nil {
				if !nodeFailureHandled(logger, node, err) {
					return nil, err
				}
				status = "degraded"
				continue
			}
			policyStatus = lookupResp.GetStatus()
			policyCoverageType = lookupResp.GetCoverageType()
		default:
			logger.Warn("unrecognized dag node service, skipping", "node", node.ID, "service", node.Service)
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
			// claimant_id_hash replaces raw claimant_name entirely here —
			// once claimant_id_hash succeeds (it's on_failure: fail_fast,
			// so reaching this point guarantees it did), there's no reason
			// for raw PII to travel any further downstream (DECISIONS.md).
			features := map[string]string{
				"tenant_id":            tenantID,
				"product":              claim.GetProduct(),
				"event_type":           claim.GetEventType(),
				"policy_number":        claim.GetPolicyNumber(),
				"claimant_id_hash":     claimantIDHash,
				"normalized_address":   normalizedAddress,
				"policy_status":        policyStatus,
				"policy_coverage_type": policyCoverageType,
				"address_valid":        strconv.FormatBool(addressValid),
			}
			resp, err := callWithResilience(ctx, cfg, node, func(ctx context.Context) (*modelv1.ScoreResponse, error) {
				return e.Model.Score(ctx, correlationID, features)
			})
			if err != nil {
				if !nodeFailureHandled(logger, node, err) {
					return nil, err
				}
				continue
			}
			scoreResp = resp
		default:
			logger.Warn("unrecognized dag node service, skipping", "node", node.ID, "service", node.Service)
		}
	}
	if scoreResp == nil {
		return nil, fmt.Errorf("dag: no model_score result produced")
	}

	// Stage 3: nodes depending on model_score (publish) — real publish to
	// claims.realtime (DESIGN.md §10). on_failure is expected to be
	// "skip" for this node (see claim.fnol.yaml's comment): the score is
	// already computed by this point, so a broker outage should degrade
	// the response rather than fail the whole request.
	for _, node := range cfg.Nodes {
		if !dependsOn(node, nodeModelScore) || !node.EnabledForTenant(tenantID) {
			continue
		}
		switch node.Service {
		case "kafka-publisher":
			event := kafka.ScoredClaimEvent{
				ClaimID:       claim.GetClaimId(),
				CorrelationID: correlationID,
				TenantID:      tenantID,
				Status:        status,
				FraudScore:    scoreResp.GetScore(),
				ModelVersion:  scoreResp.GetModelVersion(),
			}
			_, err := callWithResilience(ctx, cfg, node, func(ctx context.Context) (struct{}, error) {
				return struct{}{}, e.Publisher.Publish(ctx, event)
			})
			if err != nil {
				if !nodeFailureHandled(logger, node, err) {
					return nil, err
				}
				status = "degraded"
				continue
			}
			logger.Info("published to kafka", "topic", kafka.TopicClaimsRealtime, "claim_id", claim.GetClaimId())
		default:
			logger.Warn("unrecognized dag node service, skipping", "node", node.ID, "service", node.Service)
		}
	}

	return &Result{
		Status:            status,
		NormalizedAddress: normalizedAddress,
		FraudScore:        scoreResp.GetScore(),
		ModelVersion:      scoreResp.GetModelVersion(),
	}, nil
}

// callWithResilience runs fn under a per-attempt timeout (nodeTimeout),
// retrying up to node.MaxRetries times on any error with a fixed backoff
// between attempts. This is the enforcement side of the timeout_ms/
// max_retries fields Config/Node already carry — previously parsed from
// YAML but never actually applied to a call.
func callWithResilience[T any](ctx context.Context, cfg *Config, node Node, fn func(context.Context) (T, error)) (T, error) {
	timeout := nodeTimeout(cfg, node)
	var zero T
	var lastErr error

	for attempt := 0; attempt <= int(node.MaxRetries); attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		resp, err := fn(attemptCtx)
		cancel()
		if err == nil {
			return resp, nil
		}
		lastErr = err

		if attempt < int(node.MaxRetries) {
			select {
			case <-time.After(retryBackoff):
			case <-ctx.Done():
				return zero, ctx.Err()
			}
		}
	}
	return zero, lastErr
}

// nodeTimeout resolves the per-attempt timeout for node: its own
// timeout_ms if set, else the DAG's own timeout_ms, else
// defaultNodeTimeout.
func nodeTimeout(cfg *Config, node Node) time.Duration {
	if node.TimeoutMs > 0 {
		return time.Duration(node.TimeoutMs) * time.Millisecond
	}
	if cfg.TimeoutMs > 0 {
		return time.Duration(cfg.TimeoutMs) * time.Millisecond
	}
	return defaultNodeTimeout
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
func nodeFailureHandled(logger *slog.Logger, node Node, err error) bool {
	policy := node.EffectiveOnFailure()
	switch policy {
	case OnFailureSkip, OnFailureDegrade:
		logger.Warn("dag node failed, continuing", "node", node.ID, "on_failure", policy, "error", err)
		return true
	default: // fail_fast
		logger.Error("dag node failed", "node", node.ID, "on_failure", policy, "error", err)
		return false
	}
}
