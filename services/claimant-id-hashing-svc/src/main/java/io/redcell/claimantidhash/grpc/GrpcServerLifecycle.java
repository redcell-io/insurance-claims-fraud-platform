package io.redcell.claimantidhash.grpc;

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
 * dependency — same pattern as address-normalization-svc's
 * GrpcServerLifecycle, kept consistent across Java services rather than
 * extracted into a shared starter (only 3 services exist so far).
 */
@Component
public class GrpcServerLifecycle implements SmartLifecycle {

    private static final Logger log = LoggerFactory.getLogger(GrpcServerLifecycle.class);

    private final ClaimantIdHashingGrpcService claimantIdHashingGrpcService;
    private final int port;

    private Server server;
    private Thread awaitThread;
    private volatile boolean running = false;

    public GrpcServerLifecycle(
            ClaimantIdHashingGrpcService claimantIdHashingGrpcService,
            @Value("${app.grpc.port:9095}") int port) {
        this.claimantIdHashingGrpcService = claimantIdHashingGrpcService;
        this.port = port;
    }

    @Override
    public void start() {
        try {
            server = ServerBuilder.forPort(port)
                    .addService(claimantIdHashingGrpcService)
                    .build()
                    .start();
            running = true;
            log.info("claimant-id-hashing-svc gRPC server listening on :{}", port);
        } catch (IOException e) {
            throw new IllegalStateException("failed to start gRPC server on port " + port, e);
        }

        // This service has no embedded web server, so once
        // SpringApplication.run() returns, main() has nothing left to do —
        // without a non-daemon thread alive, the JVM exits immediately
        // even though the gRPC server "started" successfully. This thread
        // exists solely to keep the process alive until shutdown; it isn't
        // part of request handling (gRPC's own Netty threads handle that).
        // See [[java-grpc-nonweb-daemon-thread-gotcha]].
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
