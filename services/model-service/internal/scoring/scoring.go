// Package scoring is the pure fraud-risk scoring logic behind Model
// Service, kept free of gRPC so it's trivially unit-testable — same
// separation as AddressNormalizer/ClaimantIdHasher/PolicyLookup.
//
// Thin slice: a deterministic, hand-weighted formula over the feature map
// Orchestration assembles, not a real ONNX/ML model. Proves the score
// actually responds to real enrichment signals (unlike the old hardcoded
// stub) without the ONNX Runtime/trained-model-artifact/versioning
// investment — see DECISIONS.md #16 for the full rationale and what's
// deferred.
package scoring

import (
	"fmt"
	"math"
	"strconv"
)

const (
	// ModelVersion identifies this scoring implementation, distinct from
	// the old stub's "stub-v0". Bump if the weights below change in a way
	// that would meaningfully shift scores.
	ModelVersion = "rules-v1"

	baseScore = 0.05
)

// Score computes a fraud-risk score in [0, 1] from Orchestration's feature
// map, plus a human-readable explanation per triggered rule (DESIGN.md §9's
// "feature-level explanations" goal, in spirit — logged server-side by the
// caller rather than added to the proto response this pass).
//
// Missing/empty feature values are treated as their riskiest case rather
// than ignored (DESIGN.md §9: "trained-in defaults, not nulls" — a claim
// this service can't evaluate confidently should score as uncertain, not
// as automatically low-risk).
func Score(features map[string]string) (score float64, explanations []string) {
	score = baseScore

	switch features["policy_status"] {
	case "active":
		// no addition — the normal case
	case "lapsed":
		score += 0.15
		explanations = append(explanations, "policy_status=lapsed (+0.15)")
	case "cancelled":
		score += 0.25
		explanations = append(explanations, "policy_status=cancelled (+0.25)")
	default: // "unknown", "", or anything unrecognized
		score += 0.30
		explanations = append(explanations, fmt.Sprintf("policy_status=%q (+0.30)", features["policy_status"]))
	}

	if features["policy_coverage_type"] == "" {
		score += 0.05
		explanations = append(explanations, "coverage_type unknown (+0.05)")
	}

	addressValid, _ := strconv.ParseBool(features["address_valid"]) // false on missing/unparseable, which is the safe default here
	if !addressValid {
		score += 0.10
		explanations = append(explanations, "address invalid or unverified (+0.10)")
	}

	score = math.Min(1.0, math.Round(score*100)/100)

	if len(explanations) == 0 {
		explanations = []string{"no risk factors detected"}
	}
	return score, explanations
}
