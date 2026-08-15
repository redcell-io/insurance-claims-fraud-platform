package io.redcell.addressnorm.grpc;

import io.grpc.Server;
import io.grpc.ServerBuilder;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.SmartLifecycle;
import org.springframework.stereotype.Component;

import java.io.IOException;

/**
 * Starts/stops the gRPC server alongside the Spring application context.
 *
 * <p>Manual lifecycle management rather than a grpc-spring-boot-starter
 * dependency — keeps this walking-skeleton service's dependency surface
 * (and version-compat risk) minimal. Revisit if a second gRPC service
 * makes the boilerplate worth extracting into a shared starter.
 */
@Component
public class GrpcServerLifecycle implements SmartLifecycle {

    private static final Logger log = LoggerFactory.getLogger(GrpcServerLifecycle.class);

    private final AddressNormalizationGrpcService addressNormalizationGrpcService;
    private final int port;

    private Server server;
    private Thread awaitThread;
    private volatile boolean running = false;

    public GrpcServerLifecycle(
            AddressNormalizationGrpcService addressNormalizationGrpcService,
            @Value("${app.grpc.port:9092}") int port) {
        this.addressNormalizationGrpcService = addressNormalizationGrpcService;
        this.port = port;
    }

    @Override
    public void start() {
        try {
            server = ServerBuilder.forPort(port)
                    .addService(addressNormalizationGrpcService)
                    .build()
                    .start();
            running = true;
            log.info("address-normalization-svc gRPC server listening on :{}", port);
        } catch (IOException e) {
            throw new IllegalStateException("failed to start gRPC server on port " + port, e);
        }

        // This service has no embedded web server, so once
        // SpringApplication.run() returns, main() has nothing left to do —
        // without a non-daemon thread alive, the JVM exits immediately
        // even though the gRPC server "started" successfully. This thread
        // exists solely to keep the process alive until shutdown; it isn't
        // part of request handling (gRPC's own Netty threads handle that).
        awaitThread = new Thread(() -> {
            try {
                server.awaitTermination();
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            }
        }, "grpc-server-await");
        awaitThread.setDaemon(false);
        awaitThread.start();
    }

    @Override
    public void stop() {
        if (server != null) {
            server.shutdown();
        }
        running = false;
    }

    @Override
    public boolean isRunning() {
        return running;
    }

    @Override
    public int getPhase() {
        // Start after the rest of the context is up.
        return Integer.MAX_VALUE;
    }
}
