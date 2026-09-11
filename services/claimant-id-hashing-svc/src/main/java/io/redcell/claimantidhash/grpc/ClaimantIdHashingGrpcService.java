package io.redcell.claimantidhash.grpc;

import io.grpc.stub.StreamObserver;
import io.redcell.claimantidhash.ClaimantIdHasher;
import io.redcell.claimfraud.proto.claimantidhash.v1.ClaimantIdHashingServiceGrpc;
import io.redcell.claimfraud.proto.claimantidhash.v1.HashClaimantIdRequest;
import io.redcell.claimfraud.proto.claimantidhash.v1.HashClaimantIdResponse;
import org.springframework.stereotype.Component;

/**
 * gRPC adapter over {@link ClaimantIdHasher} — implements the
 * ClaimantIdHashingService contract defined in
 * proto/claimantidhash/v1/claimant_id_hashing.proto.
 */
@Component
public class ClaimantIdHashingGrpcService
        extends ClaimantIdHashingServiceGrpc.ClaimantIdHashingServiceImplBase {

    private final ClaimantIdHasher hasher = new ClaimantIdHasher();

    @Override
    public void hashClaimantId(HashClaimantIdRequest request, StreamObserver<HashClaimantIdResponse> responseObserver) {
        String hash = hasher.hash(request.getClaimantName());

        responseObserver.onNext(HashClaimantIdResponse.newBuilder()
                .setClaimantIdHash(hash)
                .build());
        responseObserver.onCompleted();
    }
}
