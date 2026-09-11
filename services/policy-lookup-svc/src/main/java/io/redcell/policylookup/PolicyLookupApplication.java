package io.redcell.policylookup;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

/**
 * Policy Lookup enrichment service. See DESIGN.md §4 item 5 and
 * DECISIONS.md for why this is a thin slice (fixture-backed, not a real
 * policy-admin-system integration).
 *
 * <p>Runs a plain Spring context (no embedded web server — this service
 * is gRPC-only) so the only listener is the gRPC server started by
 * {@link io.redcell.policylookup.grpc.GrpcServerLifecycle}.
 */
@SpringBootApplication
public class PolicyLookupApplication {

    public static void main(String[] args) {
        SpringApplication.run(PolicyLookupApplication.class, args);
    }
}
