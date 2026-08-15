package io.redcell.addressnorm.grpc;

import io.grpc.stub.StreamObserver;
import io.redcell.addressnorm.AddressNormalizer;
import io.redcell.claimfraud.proto.addressnorm.v1.AddressNormalizationServiceGrpc;
import io.redcell.claimfraud.proto.addressnorm.v1.NormalizeRequest;
import io.redcell.claimfraud.proto.addressnorm.v1.NormalizeResponse;
import org.springframework.stereotype.Component;

/**
 * gRPC adapter over {@link AddressNormalizer} — implements the
 * AddressNormalizationService contract defined in
 * proto/addressnorm/v1/address_normalization.proto.
 */
@Component
public class AddressNormalizationGrpcService
        extends AddressNormalizationServiceGrpc.AddressNormalizationServiceImplBase {

    private final AddressNormalizer normalizer = new AddressNormalizer();

    @Override
    public void normalize(NormalizeRequest request, StreamObserver<NormalizeResponse> responseObserver) {
        AddressNormalizer.Normalized result = normalizer.normalize(request.getRawAddress());

        responseObserver.onNext(NormalizeResponse.newBuilder()
                .setNormalizedAddress(result.normalizedAddress())
                .setLine1(result.line1())
                .setCity(result.city())
                .setState(result.state())
                .setPostalCode(result.postalCode())
                .setCountry(result.country())
                .setValid(result.valid())
                .build());
        responseObserver.onCompleted();
    }
}
