package dag

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"claimfraud/pkg/telemetry"

	"gopkg.in/yaml.v3"
)

// DagVersionResolver is the narrow interface Loader needs from the Tenant
// Config Service client, so it can be faked in tests without a live gRPC
// connection (mirrors AddressNormalizer/Scorer in executor.go).
type DagVersionResolver interface {
	GetDagVersion(ctx context.Context, tenantID, product string) (int32, error)
}

// Loader resolves and reads the DAG Config for a (tenantID, product,
// eventType). Thin-slice stand-in for the real DAG Config Store
// (DESIGN.md §6.1): one YAML file per (tenant, product, eventType) on disk,
// no in-memory caching, no Kafka-based invalidation — every call re-reads
// the file. Both are deferred to the same later pass as the rest of §6.1's
// caching/invalidation design.
type Loader struct {
	TenantConfig DagVersionResolver
	ConfigDir    string // e.g. "../../config/dag"
	Logger       *slog.Logger
}

// Load reads config/dag/{tenantID}/{product}/{eventType}.yaml, after
// resolving which version tenant-config-svc expects for (tenantID,
// product).
func (l *Loader) Load(ctx context.Context, tenantID, product, eventType string) (*Config, error) {
	wantVersion, err := l.TenantConfig.GetDagVersion(ctx, tenantID, product)
	if err != nil {
		return nil, fmt.Errorf("resolve dag version: %w", err)
	}

	path := filepath.Join(l.ConfigDir, tenantID, product, eventType+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read dag config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse dag config %s: %w", path, err)
	}

	if cfg.Version != wantVersion {
		// Real multi-version storage is deferred along with the rest of
		// DESIGN.md §6.1's caching/invalidation design — there's only one
		// file per (tenant,product,eventType) on disk in this pass, so a
		// mismatch just means the file is stale relative to tenant config.
		// Log and proceed with the file's own version rather than failing.
		telemetry.FromContext(ctx, l.Logger).Warn("dag config file version mismatch", "path", path, "file_version", cfg.Version, "tenant_config_version", wantVersion)
	}

	return &cfg, nil
}
