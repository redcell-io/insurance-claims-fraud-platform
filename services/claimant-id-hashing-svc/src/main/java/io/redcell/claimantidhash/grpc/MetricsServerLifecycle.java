package io.redcell.claimantidhash.grpc;

import com.sun.net.httpserver.HttpServer;
import io.micrometer.prometheusmetrics.PrometheusMeterRegistry;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.SmartLifecycle;
import org.springframework.stereotype.Component;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;

/**
 * Serves the Prometheus exposition format (DECISIONS.md #18) on its own
 * tiny HTTP listener. This service is gRPC-only otherwise (see
 * GrpcServerLifecycle's doc comment on why), so /metrics can't ride on an
 * existing embedded web server the way claims-gateway's does.
 * com.sun.net.httpserver.HttpServer is JDK built-in — avoids pulling in
 * spring-boot-starter-web/Actuator (and an embedded Tomcat) for exactly
 * one route (see DECISIONS.md #18's footprint-vs-Actuator reasoning).
 */
@Component
public class MetricsServerLifecycle implements SmartLifecycle {

    private static final Logger log = LoggerFactory.getLogger(MetricsServerLifecycle.class);

    private final PrometheusMeterRegistry registry;
    private final int port;

    private HttpServer server;
    private volatile boolean running = false;

    public MetricsServerLifecycle(
            PrometheusMeterRegistry registry,
            @Value("${app.metrics.port:9195}") int port) {
        this.registry = registry;
        this.port = port;
    }

    @Override
    public void start() {
        try {
            server = HttpServer.create(new InetSocketAddress(port), 0);
            server.createContext("/metrics", exchange -> {
                byte[] body = registry.scrape().getBytes(StandardCharsets.UTF_8);
                exchange.getResponseHeaders().add("Content-Type", "text/plain; version=0.0.4");
                exchange.sendResponseHeaders(200, body.length);
                exchange.getResponseBody().write(body);
                exchange.close();
            });
            server.start();
            running = true;
            log.info("claimant-id-hashing-svc metrics server listening on :{}", port);
        } catch (IOException e) {
            throw new IllegalStateException("failed to start metrics server on port " + port, e);
        }
    }

    @Override
    public void stop() {
        if (server != null) {
            server.stop(0);
        }
        running = false;
    }

    @Override
    public boolean isRunning() {
        return running;
    }

    @Override
    public int getPhase() {
        return Integer.MAX_VALUE;
    }
}
