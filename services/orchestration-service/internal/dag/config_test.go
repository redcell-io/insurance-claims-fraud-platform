package dag

import "testing"

func TestEffectiveOnFailureDefaultsToFailFast(t *testing.T) {
	n := Node{}
	if got := n.EffectiveOnFailure(); got != OnFailureFailFast {
		t.Errorf("EffectiveOnFailure() = %q, want %q", got, OnFailureFailFast)
	}
}

func TestEffectiveOnFailureRespectsExplicitValue(t *testing.T) {
	n := Node{OnFailure: OnFailureSkip}
	if got := n.EffectiveOnFailure(); got != OnFailureSkip {
		t.Errorf("EffectiveOnFailure() = %q, want %q", got, OnFailureSkip)
	}
}

func TestEnabledForTenantEmptyMeansAllTenants(t *testing.T) {
	n := Node{}
	if !n.EnabledForTenant("any_tenant") {
		t.Error("expected node with empty EnabledFor to be enabled for all tenants")
	}
}

func TestEnabledForTenantGatesToListedTenants(t *testing.T) {
	n := Node{EnabledFor: []string{"acme_insurance"}}
	if !n.EnabledForTenant("acme_insurance") {
		t.Error("expected acme_insurance to be enabled")
	}
	if n.EnabledForTenant("globex_insurance") {
		t.Error("expected globex_insurance to be disabled")
	}
}
