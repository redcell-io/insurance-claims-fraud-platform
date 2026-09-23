// Package telemetry provides the shared structured-logging and
// correlation-ID/tenant-ID propagation used by every Go service in this
// repo (DESIGN.md §11 — the thin-slice version: structured logging and
// gRPC interceptors, no tracing/metrics backend yet).
package telemetry

import "context"

type contextKey int

const (
	correlationIDKey contextKey = iota
	tenantIDKey
)

// WithCorrelationID returns a context carrying correlationID, readable
// back via CorrelationIDFrom — including by the logger returned from
// FromContext and by UnaryClientInterceptor, which injects it into
// outgoing gRPC metadata automatically.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationIDKey, correlationID)
}

// CorrelationIDFrom returns the correlation ID stored in ctx, or "" if
// none was set.
func CorrelationIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(correlationIDKey).(string)
	return v
}

// WithTenantID returns a context carrying tenantID, readable back via
// TenantIDFrom.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// TenantIDFrom returns the tenant ID stored in ctx, or "" if none was
// set.
func TenantIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(tenantIDKey).(string)
	return v
}
