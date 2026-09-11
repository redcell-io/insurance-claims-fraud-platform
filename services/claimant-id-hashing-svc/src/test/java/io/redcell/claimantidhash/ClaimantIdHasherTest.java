package io.redcell.claimantidhash;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class ClaimantIdHasherTest {

    private final ClaimantIdHasher hasher = new ClaimantIdHasher();

    @Test
    void hashesDeterministically() {
        assertThat(hasher.hash("Jane Doe")).isEqualTo(hasher.hash("Jane Doe"));
    }

    @Test
    void differentNamesHashDifferently() {
        assertThat(hasher.hash("Jane Doe")).isNotEqualTo(hasher.hash("John Roe"));
    }

    @Test
    void isCaseAndWhitespaceInsensitive() {
        assertThat(hasher.hash("Jane Doe")).isEqualTo(hasher.hash("  jane doe  "));
    }

    @Test
    void handlesBlankInput() {
        assertThat(hasher.hash("  ")).isEmpty();
    }
}
