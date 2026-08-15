package io.redcell.addressnorm;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

/**
 * Pure address-normalization logic, kept free of gRPC/Spring so it's
 * trivially unit-testable. Naive comma-separated parser
 * ("line1, city, state postal_code") — good enough to prove the
 * enrichment contract end-to-end for the walking skeleton; a real
 * implementation would call an address-validation provider (USPS,
 * Smarty, etc.).
 */
public class AddressNormalizer {

    public record Normalized(
            String normalizedAddress,
            String line1,
            String city,
            String state,
            String postalCode,
            String country,
            boolean valid) {
    }

    private static final String DEFAULT_COUNTRY = "US";

    public Normalized normalize(String rawAddress) {
        if (rawAddress == null || rawAddress.isBlank()) {
            return new Normalized("", "", "", "", "", "", false);
        }

        String[] parts = rawAddress.split(",");
        if (parts.length < 3) {
            // Not enough structure to parse confidently — return the
            // uppercased/trimmed input as-is and flag invalid, rather
            // than failing the call (address_normalize is on_failure:
            // skip in the DAG — degrade, don't fail_fast).
            String cleaned = rawAddress.trim().toUpperCase(Locale.ROOT);
            return new Normalized(cleaned, cleaned, "", "", "", "", false);
        }

        String line1 = parts[0].trim().toUpperCase(Locale.ROOT);
        String city = parts[1].trim().toUpperCase(Locale.ROOT);
        String[] stateZip = parts[2].trim().split("\\s+");

        String postalCode = stateZip.length > 0 ? stateZip[stateZip.length - 1].toUpperCase(Locale.ROOT) : "";
        String state = stateZip.length > 1
                ? String.join(" ", java.util.Arrays.copyOf(stateZip, stateZip.length - 1)).toUpperCase(Locale.ROOT)
                : "";

        List<String> fields = new ArrayList<>();
        fields.add(line1);
        fields.add(city);
        if (!state.isBlank()) {
            fields.add(state);
        }
        if (!postalCode.isBlank()) {
            fields.add(postalCode);
        }
        fields.add(DEFAULT_COUNTRY);

        String normalizedAddress = String.join(", ", fields);
        boolean valid = !line1.isBlank() && !city.isBlank() && !state.isBlank() && !postalCode.isBlank();

        return new Normalized(normalizedAddress, line1, city, state, postalCode, DEFAULT_COUNTRY, valid);
    }
}
