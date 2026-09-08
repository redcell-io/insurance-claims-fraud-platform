package dag

import (
	"context"
	"errors"
	"testing"

	addressnormv1 "claimfraud/proto/gen/go/addressnorm/v1"
	claimsv1 "claimfraud/proto/gen/go/claims/v1"
	modelv1 "claimfraud/proto/gen/go/model/v1"
)

type fakeConfigLoader struct {
	cfg *Config
	err error
}

func (f *fakeConfigLoader) Load(ctx context.Context, tenantID, product, eventType string) (*Config, error) {
	return f.cfg, f.err
}

type fakeAddressNorm struct {
	resp   *addressnormv1.NormalizeResponse
	err    error
	called bool
}

func (f *fakeAddressNorm) Normalize(ctx context.Context, correlationID, rawAddress string) (*addressnormv1.NormalizeResponse, error) {
	f.called = true
	return f.resp, f.err
}

type fakeScorer struct {
	resp *modelv1.ScoreResponse
	err  error
}

func (f *fakeScorer) Score(ctx context.Context, correlationID string, features map[string]string) (*modelv1.ScoreResponse, error) {
	return f.resp, f.err
}

// testConfig reproduces config/dag/acme_insurance/auto/claim.fnol.yaml's
// shape: address_normalize (skip) -> model_score (default fail_fast) ->
// publish.
func testConfig() *Config {
	return &Config{
		TenantID:  "acme_insurance",
		Product:   "auto",
		EventType: "claim.fnol",
		Version:   1,
		Nodes: []Node{
			{ID: "address_normalize", Service: "address-normalization-svc", Group: groupEnrichment, OnFailure: OnFailureSkip},
			{ID: "model_score", Service: "model-service", DependsOn: []string{groupEnrichment}},
			{ID: "publish", Service: "kafka-publisher", DependsOn: []string{nodeModelScore}},
		},
	}
}

func TestExecutorRunHappyPath(t *testing.T) {
	e := &Executor{
		Loader:      &fakeConfigLoader{cfg: testConfig()},
		AddressNorm: &fakeAddressNorm{resp: &addressnormv1.NormalizeResponse{NormalizedAddress: "123 MAIN ST"}},
		Model:       &fakeScorer{resp: &modelv1.ScoreResponse{Score: 0.42, ModelVersion: "stub-v0"}},
	}
	claim := &claimsv1.ClaimEvent{ClaimId: "clm-1", Product: "auto", EventType: "claim.fnol", RawAddress: "raw"}

	result, err := e.Run(context.Background(), "acme_insurance", "corr-1", claim)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != "scored" {
		t.Errorf("Status = %q, want %q", result.Status, "scored")
	}
	if result.NormalizedAddress != "123 MAIN ST" {
		t.Errorf("NormalizedAddress = %q, want %q", result.NormalizedAddress, "123 MAIN ST")
	}
	if result.FraudScore != 0.42 {
		t.Errorf("FraudScore = %v, want 0.42", result.FraudScore)
	}
	if result.ModelVersion != "stub-v0" {
		t.Errorf("ModelVersion = %q, want %q", result.ModelVersion, "stub-v0")
	}
}

func TestExecutorRunAddressNormalizeFailureDegradesWithSkipPolicy(t *testing.T) {
	e := &Executor{
		Loader:      &fakeConfigLoader{cfg: testConfig()},
		AddressNorm: &fakeAddressNorm{err: errors.New("address-norm unavailable")},
		Model:       &fakeScorer{resp: &modelv1.ScoreResponse{Score: 0.5, ModelVersion: "v1"}},
	}
	claim := &claimsv1.ClaimEvent{ClaimId: "clm-1", Product: "auto", EventType: "claim.fnol", RawAddress: "raw address"}

	result, err := e.Run(context.Background(), "acme_insurance", "corr-2", claim)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (on_failure=skip should not fail the run)", err)
	}
	if result.Status != "degraded" {
		t.Errorf("Status = %q, want %q", result.Status, "degraded")
	}
	if result.NormalizedAddress != "raw address" {
		t.Errorf("NormalizedAddress = %q, want fallback to raw address %q", result.NormalizedAddress, "raw address")
	}
}

func TestExecutorRunModelScoreFailureFailsFastByDefault(t *testing.T) {
	e := &Executor{
		Loader:      &fakeConfigLoader{cfg: testConfig()},
		AddressNorm: &fakeAddressNorm{resp: &addressnormv1.NormalizeResponse{NormalizedAddress: "x"}},
		Model:       &fakeScorer{err: errors.New("model-service unavailable")},
	}
	claim := &claimsv1.ClaimEvent{ClaimId: "clm-1", Product: "auto", EventType: "claim.fnol"}

	_, err := e.Run(context.Background(), "acme_insurance", "corr-3", claim)
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil (model_score has no on_failure override, defaults to fail_fast)")
	}
}

func TestExecutorRunSkipsNodeNotEnabledForTenant(t *testing.T) {
	cfg := testConfig()
	cfg.Nodes[0].EnabledFor = []string{"other_tenant"} // address_normalize gated out for acme_insurance

	fakeNorm := &fakeAddressNorm{resp: &addressnormv1.NormalizeResponse{NormalizedAddress: "should not be used"}}
	e := &Executor{
		Loader:      &fakeConfigLoader{cfg: cfg},
		AddressNorm: fakeNorm,
		Model:       &fakeScorer{resp: &modelv1.ScoreResponse{Score: 0.1, ModelVersion: "v1"}},
	}
	claim := &claimsv1.ClaimEvent{ClaimId: "clm-1", Product: "auto", EventType: "claim.fnol", RawAddress: "untouched raw address"}

	result, err := e.Run(context.Background(), "acme_insurance", "corr-4", claim)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if fakeNorm.called {
		t.Error("AddressNorm.Normalize was called for a node not enabled for this tenant")
	}
	if result.Status != "scored" {
		t.Errorf("Status = %q, want %q (gating out a node is not a failure)", result.Status, "scored")
	}
	if result.NormalizedAddress != "untouched raw address" {
		t.Errorf("NormalizedAddress = %q, want raw address unchanged", result.NormalizedAddress)
	}
}
