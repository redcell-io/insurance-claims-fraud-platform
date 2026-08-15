package io.redcell.addressnorm;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

/**
 * Address Normalization enrichment service. See DESIGN.md §4.4 and
 * build-order-plan for why this was the first Java service scaffolded.
 *
 * <p>Runs a plain Spring context (no embedded web server — this service
 * is gRPC-only) so the only listener is the gRPC server started by
 * {@link io.redcell.addressnorm.grpc.GrpcServerLifecycle}.
 */
@SpringBootApplication
public class AddressNormalizationApplication {

    public static void main(String[] args) {
        SpringApplication.run(AddressNormalizationApplication.class, args);
    }
}
