// Package store loads tenant data from a local YAML file into memory.
//
// Thin-slice stand-in for the real Postgres-backed store + transactional
// outbox described in DESIGN.md §7 — loaded once at startup, no hot-reload.
// Restart the process to pick up config changes; Kafka-based invalidation
// (`tenant-config-events`) is deferred along with everything else in that
// section.
package store

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Tenant mirrors the subset of DESIGN.md §7's tenant data model this thin
// slice needs: status gating and per-product DAG version resolution.
// api_keys/rate_limit/encryption/compliance fields are deliberately absent —
// nothing on the hot path reads them yet.
type Tenant struct {
	TenantID    string           `yaml:"tenant_id"`
	DisplayName string           `yaml:"display_name"`
	Status      string           `yaml:"status"` // active | suspended | onboarding
	DagVersions map[string]int32 `yaml:"dag_versions"` // product -> version
}

type tenantsFile struct {
	Tenants []Tenant `yaml:"tenants"`
}

// Store is an in-memory, read-only index of tenants keyed by tenant_id.
type Store struct {
	byID map[string]Tenant
}

// Load reads and parses the tenants YAML file at path.
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tenants file: %w", err)
	}

	var tf tenantsFile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parse tenants file: %w", err)
	}

	byID := make(map[string]Tenant, len(tf.Tenants))
	for _, t := range tf.Tenants {
		byID[t.TenantID] = t
	}
	return &Store{byID: byID}, nil
}

// Get returns the tenant for tenantID, if known.
func (s *Store) Get(tenantID string) (Tenant, bool) {
	t, ok := s.byID[tenantID]
	return t, ok
}

// DagVersion returns the configured DAG version for (tenantID, product).
func (s *Store) DagVersion(tenantID, product string) (int32, bool) {
	t, ok := s.byID[tenantID]
	if !ok {
		return 0, false
	}
	v, ok := t.DagVersions[product]
	return v, ok
}
