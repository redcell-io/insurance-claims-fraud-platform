package io.redcell.policylookup;

import java.util.Locale;
import java.util.Map;

/**
 * Pure policy-lookup logic, kept free of gRPC/Spring so it's trivially
 * unit-testable. Fixture-backed, in-memory known policies — good enough
 * to prove the enrichment contract end-to-end; a real implementation
 * would call the policy-admin system.
 */
public class PolicyLookup {

    public record Result(boolean found, String status, String coverageType) {
        static Result notFound() {
            return new Result(false, "unknown", "");
        }
    }

    // Fixture data standing in for a real policy-admin-system integration
    // (DESIGN.md §4 item 5) — see DECISIONS.md. POL-123456 is the policy
    // number used in RUNBOOK.md's smoke test.
    private static final Map<String, Result> KNOWN_POLICIES = Map.of(
            "POL-123456", new Result(true, "active", "full"),
            "POL-654321", new Result(true, "lapsed", "liability_only"),
            "POL-000000", new Result(true, "cancelled", "full")
    );

    public Result lookup(String policyNumber) {
        if (policyNumber == null || policyNumber.isBlank()) {
            return Result.notFound();
        }
        return KNOWN_POLICIES.getOrDefault(policyNumber.trim().toUpperCase(Locale.ROOT), Result.notFound());
    }
}
