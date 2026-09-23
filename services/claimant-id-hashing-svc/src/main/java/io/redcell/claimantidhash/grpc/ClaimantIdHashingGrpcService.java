package io.redcell.claimantidhash.grpc;

import io.grpc.stub.StreamObserver;
import io.redcell.claimantidhash.ClaimantIdHasher;
import io.redcell.claimfraud.proto.claimantidhash.v1.ClaimantIdHashingServiceGrpc;
import io.redcell.claimfraud.proto.claimantidhash.v1.HashClaimantIdRequest;
import io.redcell.claimfraud.proto.claimantidhash.v1.HashClaimantIdResponse;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.stereotype.Component;

/**
 * gRPC adapter over {@link ClaimantIdHasher} — implements the
 * ClaimantIdHashingService contract defined in
 * proto/claimantidhash/v1/claimant_id_hashing.proto.
 */
@Component
public class ClaimantIdHashingGrpcService
        extends ClaimantIdHashingServiceGrpc.ClaimantIdHashingServiceImplBase {

    private static final Logger log = LoggerFactory.getLogger(ClaimantIdHashingGrpcService.class);

    private final ClaimantIdHasher hasher = new ClaimantIdHasher();

    @Override
    public void hashClaimantId(HashClaimantIdRequest request, StreamObserver<HashClaimantIdResponse> responseObserver) {
        MDC.put("correlation_id", request.getCorrelationId());
        try {
            log.info("hashClaimantId request received");
            String hash = hasher.hash(request.getClaimantName());

            responseObserver.onNext(HashClaimantIdResponse.newBuilder()
                    .setClaimantIdHash(hash)
                    .build());
            responseObserver.onCompleted();
            log.info("hashClaimantId request completed");
        } finally {
            MDC.remove("correlation_id");
        }
    }
}
