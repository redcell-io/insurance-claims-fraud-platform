package io.redcell.claimantidhash;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.HexFormat;
import java.util.Locale;

/**
 * Pure claimant-ID hashing logic, kept free of gRPC/Spring so it's
 * trivially unit-testable. SHA-256 of a static salt + normalized claimant
 * name — good enough to prove the hash-not-raw-PII contract end-to-end; a
 * real implementation would derive the salt/key per tenant from KMS
 * (DESIGN.md §12), not use one static value baked into the jar.
 */
public class ClaimantIdHasher {

    // Placeholder salt — NOT a substitute for real per-tenant KMS-derived
    // keys (DESIGN.md §12). Static and baked into the binary purely to
    // prove the "hash instead of raw PII" contract for the walking
    // skeleton; see DECISIONS.md.
    private static final String STATIC_SALT = "claim-fraud-platform-dev-salt-v1";

    public String hash(String claimantName) {
        if (claimantName == null || claimantName.isBlank()) {
            return "";
        }

        // Case/whitespace-normalize first so "Jane Doe" and "jane doe"
        // resolve to the same claimant — the whole point of this hash is
        // matching the same claimant across differently-formatted
        // submissions.
        String normalized = claimantName.trim().toLowerCase(Locale.ROOT);

        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            byte[] hashed = digest.digest((STATIC_SALT + normalized).getBytes(StandardCharsets.UTF_8));
            return HexFormat.of().formatHex(hashed);
        } catch (NoSuchAlgorithmException e) {
            // SHA-256 is guaranteed available on every JVM — unreachable.
            throw new IllegalStateException("SHA-256 not available", e);
        }
    }
}
