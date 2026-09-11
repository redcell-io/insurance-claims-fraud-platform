package io.redcell.claimantidhash;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

/**
 * Claimant ID Hashing enrichment service. See DESIGN.md §4 item 3 and
 * DECISIONS.md for why this is a thin slice (static salt, not per-tenant
 * KMS keys).
 *
 * <p>Runs a plain Spring context (no embedded web server — this service
 * is gRPC-only) so the only listener is the gRPC server started by
 * {@link io.redcell.claimantidhash.grpc.GrpcServerLifecycle}.
 */
@SpringBootApplication
public class ClaimantIdHashingApplication {

    public static void main(String[] args) {
        SpringApplication.run(ClaimantIdHashingApplication.class, args);
    }
}
