package io.redcell.policylookup;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class PolicyLookupTest {

    private final PolicyLookup lookup = new PolicyLookup();

    @Test
    void findsKnownActivePolicy() {
        PolicyLookup.Result result = lookup.lookup("POL-123456");
        assertThat(result.found()).isTrue();
        assertThat(result.status()).isEqualTo("active");
        assertThat(result.coverageType()).isEqualTo("full");
    }

    @Test
    void findsKnownLapsedPolicy() {
        PolicyLookup.Result result = lookup.lookup("POL-654321");
        assertThat(result.found()).isTrue();
        assertThat(result.status()).isEqualTo("lapsed");
    }

    @Test
    void returnsNotFoundForUnknownPolicy() {
        PolicyLookup.Result result = lookup.lookup("POL-DOES-NOT-EXIST");
        assertThat(result.found()).isFalse();
        assertThat(result.status()).isEqualTo("unknown");
    }

    @Test
    void handlesBlankInput() {
        assertThat(lookup.lookup("  ").found()).isFalse();
    }

    @Test
    void isCaseInsensitive() {
        assertThat(lookup.lookup("pol-123456").found()).isTrue();
    }
}
