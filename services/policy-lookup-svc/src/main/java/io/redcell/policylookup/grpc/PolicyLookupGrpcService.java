package io.redcell.policylookup.grpc;

import io.grpc.stub.StreamObserver;
import io.redcell.claimfraud.proto.policylookup.v1.LookupPolicyRequest;
import io.redcell.claimfraud.proto.policylookup.v1.LookupPolicyResponse;
import io.redcell.claimfraud.proto.policylookup.v1.PolicyLookupServiceGrpc;
import io.redcell.policylookup.PolicyLookup;
import org.springframework.stereotype.Component;

/**
 * gRPC adapter over {@link PolicyLookup} — implements the
 * PolicyLookupService contract defined in
 * proto/policylookup/v1/policy_lookup.proto.
 */
@Component
public class PolicyLookupGrpcService extends PolicyLookupServiceGrpc.PolicyLookupServiceImplBase {

    private final PolicyLookup policyLookup = new PolicyLookup();

    @Override
    public void lookupPolicy(LookupPolicyRequest request, StreamObserver<LookupPolicyResponse> responseObserver) {
        PolicyLookup.Result result = policyLookup.lookup(request.getPolicyNumber());

        responseObserver.onNext(LookupPolicyResponse.newBuilder()
                .setFound(result.found())
                .setStatus(result.status())
                .setCoverageType(result.coverageType())
                .build());
        responseObserver.onCompleted();
    }
}
