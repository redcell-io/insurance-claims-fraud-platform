package telemetry

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RED metrics (rate/errors/duration — DESIGN.md §11) for every gRPC call
// this service makes or handles, both directions. No "service" label:
// each service exposes these on its own /metrics endpoint, and
// Prometheus's own scrape `job` label (config/prometheus.yml) already
// distinguishes which service a sample came from — a redundant
// in-metric label would just be the same information twice.
//
// tenant_id as a label is DESIGN.md §11's explicit requirement
// ("tagged with tenant_id for per-carrier slicing"). Safe at today's
// scale (2 tenants in config/tenants.yaml); revisit if tenant count
// ever grows into the hundreds+ (Prometheus label cardinality is
// per-metric-name multiplicative across all label combinations).
var (
	requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_requests_total",
		Help: "Total gRPC calls, both server-handled and client-made.",
	}, []string{"method", "tenant_id", "status"})

	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "grpc_request_duration_seconds",
		Help:    "gRPC call duration in seconds, both server-handled and client-made.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "tenant_id"})
)

// RecordRequest is called once per gRPC call, by both
// UnaryServerInterceptor and UnaryClientInterceptor — they already
// compute duration and know method/tenant_id/error state, so this is
// additive to existing interceptor code, not a new instrumentation
// surface. status is "ok" or "error"; a code-level breakdown
// (NotFound/Unavailable/etc.) is deliberately not a label here to keep
// cardinality bounded — the structured logs (pkg/telemetry's logging.go)
// already carry the real error for debugging a specific failure.
func RecordRequest(method, tenantID string, ok bool, d time.Duration) {
	status := "ok"
	if !ok {
		status = "error"
	}
	requestsTotal.WithLabelValues(method, tenantID, status).Inc()
	requestDuration.WithLabelValues(method, tenantID).Observe(d.Seconds())
}

// MetricsHandler serves the Prometheus exposition format — mount at
// /metrics. claims-gateway mounts this on its existing http.ServeMux;
// the gRPC-only services (tenant-config-svc, model-service,
// orchestration-service) start a small second HTTP listener for it,
// since they have no HTTP server otherwise.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}
