package io.redcell.policylookup.grpc;

import io.grpc.stub.StreamObserver;
import io.redcell.claimfraud.proto.policylookup.v1.LookupPolicyRequest;
import io.redcell.claimfraud.proto.policylookup.v1.LookupPolicyResponse;
import io.redcell.claimfraud.proto.policylookup.v1.PolicyLookupServiceGrpc;
import io.redcell.policylookup.PolicyLookup;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.stereotype.Component;

/**
 * gRPC adapter over {@link PolicyLookup} — implements the
 * PolicyLookupService contract defined in
 * proto/policylookup/v1/policy_lookup.proto.
 */
@Component
public class PolicyLookupGrpcService extends PolicyLookupServiceGrpc.PolicyLookupServiceImplBase {

    private static final Logger log = LoggerFactory.getLogger(PolicyLookupGrpcService.class);

    private final PolicyLookup policyLookup = new PolicyLookup();

    @Override
    public void lookupPolicy(LookupPolicyRequest request, StreamObserver<LookupPolicyResponse> responseObserver) {
        MDC.put("correlation_id", request.getCorrelationId());
        try {
            log.info("lookupPolicy request received");
            PolicyLookup.Result result = policyLookup.lookup(request.getPolicyNumber());

            responseObserver.onNext(LookupPolicyResponse.newBuilder()
                    .setFound(result.found())
                    .setStatus(result.status())
                    .setCoverageType(result.coverageType())
                    .build());
            responseObserver.onCompleted();
            log.info("lookupPolicy request completed, found={}, status={}", result.found(), result.status());
        } finally {
            MDC.remove("correlation_id");
        }
    }
}
