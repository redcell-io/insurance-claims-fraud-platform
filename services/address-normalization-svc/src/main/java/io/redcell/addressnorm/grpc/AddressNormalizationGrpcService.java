package io.redcell.addressnorm.grpc;

import io.grpc.stub.StreamObserver;
import io.redcell.addressnorm.AddressNormalizer;
import io.redcell.claimfraud.proto.addressnorm.v1.AddressNormalizationServiceGrpc;
import io.redcell.claimfraud.proto.addressnorm.v1.NormalizeRequest;
import io.redcell.claimfraud.proto.addressnorm.v1.NormalizeResponse;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.stereotype.Component;

/**
 * gRPC adapter over {@link AddressNormalizer} — implements the
 * AddressNormalizationService contract defined in
 * proto/addressnorm/v1/address_normalization.proto.
 */
@Component
public class AddressNormalizationGrpcService
        extends AddressNormalizationServiceGrpc.AddressNormalizationServiceImplBase {

    private static final Logger log = LoggerFactory.getLogger(AddressNormalizationGrpcService.class);

    private final AddressNormalizer normalizer = new AddressNormalizer();

    @Override
    public void normalize(NormalizeRequest request, StreamObserver<NormalizeResponse> responseObserver) {
        MDC.put("correlation_id", request.getCorrelationId());
        try {
            log.info("normalize request received");
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
            log.info("normalize request completed, valid={}", result.valid());
        } finally {
            MDC.remove("correlation_id");
        }
    }
}
