package dag

import (
	"context"
	"errors"
	"testing"
	"time"

	addressnormv1 "claimfraud/proto/gen/go/addressnorm/v1"
	claimantidhashv1 "claimfraud/proto/gen/go/claimantidhash/v1"
	claimsv1 "claimfraud/proto/gen/go/claims/v1"
	modelv1 "claimfraud/proto/gen/go/model/v1"
	policylookupv1 "claimfraud/proto/gen/go/policylookup/v1"
	"claimfraud/services/orchestration-service/internal/kafka"
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

type fakeClaimantIDHasher struct {
	resp   *claimantidhashv1.HashClaimantIdResponse
	err    error
	called bool
}

func (f *fakeClaimantIDHasher) Hash(ctx context.Context, correlationID, claimantName string) (*claimantidhashv1.HashClaimantIdResponse, error) {
	f.called = true
	return f.resp, f.err
}

type fakePolicyLookup struct {
	resp   *policylookupv1.LookupPolicyResponse
	err    error
	called bool
}

func (f *fakePolicyLookup) Lookup(ctx context.Context, correlationID, policyNumber string) (*policylookupv1.LookupPolicyResponse, error) {
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

type fakePublisher struct {
	err     error
	called  bool
	lastMsg kafka.ScoredClaimEvent
}

func (f *fakePublisher) Publish(ctx context.Context, event kafka.ScoredClaimEvent) error {
	f.called = true
	f.lastMsg = event
	return f.err
}

// testConfig reproduces config/dag/acme_insurance/auto/claim.fnol.yaml's
// v3 shape: claimant_id_hash (fail_fast) + address_normalize (skip) +
// policy_lookup (degrade) [enrichment group] -> model_score (default
// fail_fast) -> publish (skip).
func testConfig() *Config {
	return &Config{
		TenantID:  "acme_insurance",
		Product:   "auto",
		EventType: "claim.fnol",
		Version:   3,
		Nodes: []Node{
			{ID: "claimant_id_hash", Service: "claimant-id-hashing-svc", Group: groupEnrichment, OnFailure: OnFailureFailFast},
			{ID: "address_normalize", Service: "address-normalization-svc", Group: groupEnrichment, OnFailure: OnFailureSkip},
			{ID: "policy_lookup", Service: "policy-lookup-svc", Group: groupEnrichment, OnFailure: OnFailureDegrade},
			{ID: "model_score", Service: "model-service", DependsOn: []string{groupEnrichment}},
			{ID: "publish", Service: "kafka-publisher", DependsOn: []string{nodeModelScore}, OnFailure: OnFailureSkip},
		},
	}
}

// nodeByID finds a node by id, failing the test if it's not present —
// keeps tests resilient to reordering testConfig()'s node list.
func nodeByID(t *testing.T, cfg *Config, id string) *Node {
	t.Helper()
	for i := range cfg.Nodes {
		if cfg.Nodes[i].ID == id {
			return &cfg.Nodes[i]
		}
	}
	t.Fatalf("no node with id %q in testConfig()", id)
	return nil
}

// successfulClaimantIDHash and successfulPolicyLookup are the default
// "everything upstream worked" fakes, for tests that aren't specifically
// exercising those two nodes.
func successfulClaimantIDHash() *fakeClaimantIDHasher {
	return &fakeClaimantIDHasher{resp: &claimantidhashv1.HashClaimantIdResponse{ClaimantIdHash: "deadbeef"}}
}

func successfulPolicyLookup() *fakePolicyLookup {
	return &fakePolicyLookup{resp: &policylookupv1.LookupPolicyResponse{Found: true, Status: "active", CoverageType: "full"}}
}

func successfulPublisher() *fakePublisher {
	return &fakePublisher{}
}

func TestExecutorRunHappyPath(t *testing.T) {
	e := &Executor{
		Loader:         &fakeConfigLoader{cfg: testConfig()},
		AddressNorm:    &fakeAddressNorm{resp: &addressnormv1.NormalizeResponse{NormalizedAddress: "123 MAIN ST"}},
		ClaimantIDHash: successfulClaimantIDHash(),
		PolicyLookup:   successfulPolicyLookup(),
		Model:          &fakeScorer{resp: &modelv1.ScoreResponse{Score: 0.42, ModelVersion: "stub-v0"}},
		Publisher:      successfulPublisher(),
	}
	claim := &claimsv1.ClaimEvent{ClaimId: "clm-1", Product: "auto", EventType: "claim.fnol", RawAddress: "raw", ClaimantName: "Jane Doe", PolicyNumber: "POL-123456"}

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
		Loader:         &fakeConfigLoader{cfg: testConfig()},
		AddressNorm:    &fakeAddressNorm{err: errors.New("address-norm unavailable")},
		ClaimantIDHash: successfulClaimantIDHash(),
		PolicyLookup:   successfulPolicyLookup(),
		Model:          &fakeScorer{resp: &modelv1.ScoreResponse{Score: 0.5, ModelVersion: "v1"}},
		Publisher:      successfulPublisher(),
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
		Loader:         &fakeConfigLoader{cfg: testConfig()},
		AddressNorm:    &fakeAddressNorm{resp: &addressnormv1.NormalizeResponse{NormalizedAddress: "x"}},
		ClaimantIDHash: successfulClaimantIDHash(),
		PolicyLookup:   successfulPolicyLookup(),
		Model:          &fakeScorer{err: errors.New("model-service unavailable")},
		Publisher:      successfulPublisher(),
	}
	claim := &claimsv1.ClaimEvent{ClaimId: "clm-1", Product: "auto", EventType: "claim.fnol"}

	_, err := e.Run(context.Background(), "acme_insurance", "corr-3", claim)
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil (model_score has no on_failure override, defaults to fail_fast)")
	}
}

func TestExecutorRunClaimantIdHashFailureFailsFast(t *testing.T) {
	e := &Executor{
		Loader:         &fakeConfigLoader{cfg: testConfig()},
		AddressNorm:    &fakeAddressNorm{resp: &addressnormv1.NormalizeResponse{NormalizedAddress: "x"}},
		ClaimantIDHash: &fakeClaimantIDHasher{err: errors.New("claimant-id-hashing-svc unavailable")},
		PolicyLookup:   successfulPolicyLookup(),
		Model:          &fakeScorer{resp: &modelv1.ScoreResponse{Score: 0.1, ModelVersion: "v1"}},
		Publisher:      successfulPublisher(),
	}
	claim := &claimsv1.ClaimEvent{ClaimId: "clm-1", Product: "auto", EventType: "claim.fnol"}

	_, err := e.Run(context.Background(), "acme_insurance", "corr-5", claim)
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil (claimant_id_hash is on_failure: fail_fast)")
	}
}

func TestExecutorRunPolicyNotFoundStillScores(t *testing.T) {
	e := &Executor{
		Loader:         &fakeConfigLoader{cfg: testConfig()},
		AddressNorm:    &fakeAddressNorm{resp: &addressnormv1.NormalizeResponse{NormalizedAddress: "x"}},
		ClaimantIDHash: successfulClaimantIDHash(),
		// found=false is a normal business result, not an RPC error — the
		// call itself succeeds.
		PolicyLookup: &fakePolicyLookup{resp: &policylookupv1.LookupPolicyResponse{Found: false, Status: "unknown"}},
		Model:        &fakeScorer{resp: &modelv1.ScoreResponse{Score: 0.9, ModelVersion: "v1"}},
		Publisher:    successfulPublisher(),
	}
	claim := &claimsv1.ClaimEvent{ClaimId: "clm-1", Product: "auto", EventType: "claim.fnol", PolicyNumber: "POL-DOES-NOT-EXIST"}

	result, err := e.Run(context.Background(), "acme_insurance", "corr-6", claim)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (found=false is not a call failure)", err)
	}
	if result.Status != "scored" {
		t.Errorf("Status = %q, want %q (an unknown policy number is not a degrade condition)", result.Status, "scored")
	}
}

func TestExecutorRunPublishesScoredEvent(t *testing.T) {
	pub := successfulPublisher()
	e := &Executor{
		Loader:         &fakeConfigLoader{cfg: testConfig()},
		AddressNorm:    &fakeAddressNorm{resp: &addressnormv1.NormalizeResponse{NormalizedAddress: "123 MAIN ST"}},
		ClaimantIDHash: successfulClaimantIDHash(),
		PolicyLookup:   successfulPolicyLookup(),
		Model:          &fakeScorer{resp: &modelv1.ScoreResponse{Score: 0.42, ModelVersion: "stub-v0"}},
		Publisher:      pub,
	}
	claim := &claimsv1.ClaimEvent{ClaimId: "clm-7", Product: "auto", EventType: "claim.fnol"}

	if _, err := e.Run(context.Background(), "acme_insurance", "corr-7", claim); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !pub.called {
		t.Fatal("Publisher.Publish was not called")
	}
	want := kafka.ScoredClaimEvent{
		ClaimID:       "clm-7",
		CorrelationID: "corr-7",
		TenantID:      "acme_insurance",
		Status:        "scored",
		FraudScore:    0.42,
		ModelVersion:  "stub-v0",
	}
	if pub.lastMsg != want {
		t.Errorf("published event = %+v, want %+v", pub.lastMsg, want)
	}
}

func TestExecutorRunPublishFailureDegradesWithSkipPolicy(t *testing.T) {
	e := &Executor{
		Loader:         &fakeConfigLoader{cfg: testConfig()},
		AddressNorm:    &fakeAddressNorm{resp: &addressnormv1.NormalizeResponse{NormalizedAddress: "x"}},
		ClaimantIDHash: successfulClaimantIDHash(),
		PolicyLookup:   successfulPolicyLookup(),
		Model:          &fakeScorer{resp: &modelv1.ScoreResponse{Score: 0.42, ModelVersion: "stub-v0"}},
		Publisher:      &fakePublisher{err: errors.New("broker unavailable")},
	}
	claim := &claimsv1.ClaimEvent{ClaimId: "clm-8", Product: "auto", EventType: "claim.fnol"}

	result, err := e.Run(context.Background(), "acme_insurance", "corr-8", claim)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (publish's on_failure=skip should not fail the run)", err)
	}
	if result.Status != "degraded" {
		t.Errorf("Status = %q, want %q (a failed publish degrades the response, per claim.fnol.yaml's on_failure: skip on the publish node)", result.Status, "degraded")
	}
	// The client still gets its score even though the publish failed —
	// that's the whole point of on_failure: skip here.
	if result.FraudScore != 0.42 {
		t.Errorf("FraudScore = %v, want 0.42 (a publish failure shouldn't discard an already-computed score)", result.FraudScore)
	}
}

func TestExecutorRunSkipsNodeNotEnabledForTenant(t *testing.T) {
	cfg := testConfig()
	nodeByID(t, cfg, "address_normalize").EnabledFor = []string{"other_tenant"} // gated out for acme_insurance

	fakeNorm := &fakeAddressNorm{resp: &addressnormv1.NormalizeResponse{NormalizedAddress: "should not be used"}}
	e := &Executor{
		Loader:         &fakeConfigLoader{cfg: cfg},
		AddressNorm:    fakeNorm,
		ClaimantIDHash: successfulClaimantIDHash(),
		PolicyLookup:   successfulPolicyLookup(),
		Model:          &fakeScorer{resp: &modelv1.ScoreResponse{Score: 0.1, ModelVersion: "v1"}},
		Publisher:      successfulPublisher(),
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

// slowAddressNorm blocks until its ctx is cancelled — used to prove a
// node's timeout_ms is actually enforced (callWithResilience) rather
// than parsed-and-ignored, which was the case before this pass.
type slowAddressNorm struct {
	callCount int
}

func (f *slowAddressNorm) Normalize(ctx context.Context, correlationID, rawAddress string) (*addressnormv1.NormalizeResponse, error) {
	f.callCount++
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestExecutorRunNodeTimeoutIsEnforced(t *testing.T) {
	cfg := testConfig()
	nodeByID(t, cfg, "address_normalize").TimeoutMs = 5 // small so the test stays fast

	slow := &slowAddressNorm{}
	e := &Executor{
		Loader:         &fakeConfigLoader{cfg: cfg},
		AddressNorm:    slow,
		ClaimantIDHash: successfulClaimantIDHash(),
		PolicyLookup:   successfulPolicyLookup(),
		Model:          &fakeScorer{resp: &modelv1.ScoreResponse{Score: 0.1, ModelVersion: "v1"}},
		Publisher:      successfulPublisher(),
	}
	claim := &claimsv1.ClaimEvent{ClaimId: "clm-1", Product: "auto", EventType: "claim.fnol", RawAddress: "raw"}

	start := time.Now()
	result, err := e.Run(context.Background(), "acme_insurance", "corr-timeout", claim)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run() error = %v, want nil (address_normalize's on_failure=skip should not fail the run)", err)
	}
	if result.Status != "degraded" {
		t.Errorf("Status = %q, want %q (a timed-out node should degrade the result)", result.Status, "degraded")
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("Run() took %v, want well under 200ms — a node with timeout_ms=5 should bound the call instead of blocking forever", elapsed)
	}
	if slow.callCount != 1 {
		t.Errorf("Normalize called %d times, want 1 (testConfig's address_normalize node has max_retries=0)", slow.callCount)
	}
}

// flakyPolicyLookup fails a fixed number of times before succeeding —
// used to prove max_retries actually retries rather than giving up on
// the first transient error.
type flakyPolicyLookup struct {
	failuresBeforeSuccess int
	callCount             int
	successResp           *policylookupv1.LookupPolicyResponse
}

func (f *flakyPolicyLookup) Lookup(ctx context.Context, correlationID, policyNumber string) (*policylookupv1.LookupPolicyResponse, error) {
	f.callCount++
	if f.callCount <= f.failuresBeforeSuccess {
		return nil, errors.New("transient failure")
	}
	return f.successResp, nil
}

func TestExecutorRunRetriesTransientFailureIntoSuccess(t *testing.T) {
	cfg := testConfig()
	nodeByID(t, cfg, "policy_lookup").MaxRetries = 2

	flaky := &flakyPolicyLookup{
		failuresBeforeSuccess: 2,
		successResp:           &policylookupv1.LookupPolicyResponse{Found: true, Status: "active", CoverageType: "full"},
	}
	e := &Executor{
		Loader:         &fakeConfigLoader{cfg: cfg},
		AddressNorm:    &fakeAddressNorm{resp: &addressnormv1.NormalizeResponse{NormalizedAddress: "x"}},
		ClaimantIDHash: successfulClaimantIDHash(),
		PolicyLookup:   flaky,
		Model:          &fakeScorer{resp: &modelv1.ScoreResponse{Score: 0.1, ModelVersion: "v1"}},
		Publisher:      successfulPublisher(),
	}
	claim := &claimsv1.ClaimEvent{ClaimId: "clm-1", Product: "auto", EventType: "claim.fnol", PolicyNumber: "POL-123456"}

	result, err := e.Run(context.Background(), "acme_insurance", "corr-retry", claim)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (should succeed after retries exhaust the transient failures)", err)
	}
	if result.Status != "scored" {
		t.Errorf("Status = %q, want %q (a retry that eventually succeeds should not degrade the result)", result.Status, "scored")
	}
	if flaky.callCount != 3 {
		t.Errorf("Lookup called %d times, want 3 (2 failures + 1 success, max_retries=2)", flaky.callCount)
	}
}
