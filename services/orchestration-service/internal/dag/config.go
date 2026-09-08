// Package dag executes the enrichment DAG for a claim, driven by a
// per-(tenant, product, event_type) YAML config (DESIGN.md §6.1) instead
// of a hardcoded sequence — see loader.go and executor.go.
package dag

// Config mirrors DESIGN.md §6.1's YAML shape 1:1.
type Config struct {
	TenantID  string `yaml:"tenant_id"`
	Product   string `yaml:"product"`
	EventType string `yaml:"event_type"`
	Version   int32  `yaml:"version"`
	TimeoutMs int32  `yaml:"timeout_ms"`
	Nodes     []Node `yaml:"nodes"`
}

// Node is a single DAG node. Nodes with no DependsOn belong to the implicit
// "enrichment" stage and run concurrently; nodes that depend on a Group name
// run after every node in that group has finished (see executor.go — this
// is a *staged* model, not a fully general topological sort, which is all
// DESIGN.md §6.1's example DAG shape actually needs today).
type Node struct {
	ID         string   `yaml:"id"`
	Service    string   `yaml:"service"`
	Group      string   `yaml:"group"`
	TimeoutMs  int32    `yaml:"timeout_ms"`
	OnFailure  string   `yaml:"on_failure"` // fail_fast | skip | degrade, default fail_fast
	DependsOn  []string `yaml:"depends_on"`
	EnabledFor []string `yaml:"enabled_for"` // empty = enabled for all tenants
}

const (
	OnFailureFailFast = "fail_fast"
	OnFailureSkip     = "skip"
	OnFailureDegrade  = "degrade"
)

// EffectiveOnFailure returns the node's on_failure policy, defaulting to
// fail_fast when unset (DESIGN.md §6.1: model_score has no override in the
// example DAG, i.e. it takes the DAG-level default).
func (n Node) EffectiveOnFailure() string {
	if n.OnFailure == "" {
		return OnFailureFailFast
	}
	return n.OnFailure
}

// EnabledForTenant reports whether the node applies to tenantID — empty
// EnabledFor means "all tenants" (DESIGN.md §6.1's `enabled_for` gates
// tenant-specific nodes without forking the whole DAG).
func (n Node) EnabledForTenant(tenantID string) bool {
	if len(n.EnabledFor) == 0 {
		return true
	}
	for _, t := range n.EnabledFor {
		if t == tenantID {
			return true
		}
	}
	return false
}
