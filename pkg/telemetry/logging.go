package telemetry

import (
	"context"
	"log/slog"
	"os"
)

// NewLogger returns a JSON logger for serviceName, writing to stdout.
// One of these is created once per service at startup and threaded
// through to FromContext.
func NewLogger(serviceName string) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, nil)
	return slog.New(handler).With("service", serviceName)
}

// FromContext returns logger with correlation_id/tenant_id attributes
// attached whenever ctx carries them (populated by UnaryServerInterceptor
// on the way in, or set directly via WithCorrelationID/WithTenantID).
// Callers that don't have a context yet (e.g. at startup) should just use
// logger directly. A nil logger falls back to slog.Default() — keeps
// tests/fakes that construct a struct without a Logger field from
// nil-panicking rather than requiring every call site to set one.
func FromContext(ctx context.Context, logger *slog.Logger) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	if id := CorrelationIDFrom(ctx); id != "" {
		logger = logger.With("correlation_id", id)
	}
	if id := TenantIDFrom(ctx); id != "" {
		logger = logger.With("tenant_id", id)
	}
	return logger
}
