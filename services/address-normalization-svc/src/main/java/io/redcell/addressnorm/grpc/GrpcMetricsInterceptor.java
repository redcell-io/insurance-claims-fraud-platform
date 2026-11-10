package io.redcell.addressnorm.grpc;

import io.grpc.ForwardingServerCall.SimpleForwardingServerCall;
import io.grpc.Metadata;
import io.grpc.ServerCall;
import io.grpc.ServerCallHandler;
import io.grpc.ServerInterceptor;
import io.grpc.Status;
import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.Timer;

import java.util.concurrent.TimeUnit;

/**
 * Records RED metrics (DECISIONS.md #18) for every gRPC call this service
 * handles — same two metric names/labels as pkg/telemetry's Go-side
 * RecordRequest (grpc_requests_total, grpc_request_duration_seconds), so
 * one Grafana dashboard/query works across all 7 services regardless of
 * language. tenant_id is read from the "x-tenant-id" gRPC metadata
 * header, which the Go client interceptor already attaches on every
 * downstream call regardless of the callee's language (see
 * pkg/telemetry/interceptor.go) — this service never has to parse it out
 * of a proto field.
 *
 * <p>Status is captured in {@code close()}, which gRPC calls exactly once
 * per call regardless of success/failure/cancellation — the Java
 * equivalent of wrapping the Go interceptor's single handler(ctx, req)
 * call.
 */
public class GrpcMetricsInterceptor implements ServerInterceptor {

    private static final Metadata.Key<String> TENANT_ID_KEY =
            Metadata.Key.of("x-tenant-id", Metadata.ASCII_STRING_MARSHALLER);

    private final MeterRegistry registry;

    public GrpcMetricsInterceptor(MeterRegistry registry) {
        this.registry = registry;
    }

    @Override
    public <ReqT, RespT> ServerCall.Listener<ReqT> interceptCall(
            ServerCall<ReqT, RespT> call, Metadata headers, ServerCallHandler<ReqT, RespT> next) {
        String method = call.getMethodDescriptor().getFullMethodName();
        String rawTenantId = headers.get(TENANT_ID_KEY);
        String tenantId = rawTenantId != null ? rawTenantId : "";
        long startNanos = System.nanoTime();

        ServerCall<ReqT, RespT> wrapped = new SimpleForwardingServerCall<>(call) {
            @Override
            public void close(Status status, Metadata trailers) {
                record(method, tenantId, status, startNanos);
                super.close(status, trailers);
            }
        };
        return next.startCall(wrapped, headers);
    }

    private void record(String method, String tenantId, Status status, long startNanos) {
        String outcome = status.isOk() ? "ok" : "error";
        registry.counter("grpc_requests_total", "method", method, "tenant_id", tenantId, "status", outcome)
                .increment();
        Timer.builder("grpc_request_duration_seconds")
                .tag("method", method)
                .tag("tenant_id", tenantId)
                .publishPercentileHistogram()
                .register(registry)
                .record(System.nanoTime() - startNanos, TimeUnit.NANOSECONDS);
    }
}
