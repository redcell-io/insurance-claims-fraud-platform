package io.redcell.addressnorm;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class AddressNormalizerTest {

    private final AddressNormalizer normalizer = new AddressNormalizer();

    @Test
    void normalizesWellFormedAddress() {
        AddressNormalizer.Normalized result =
                normalizer.normalize("123 Main St, Springfield, IL 62704");

        assertThat(result.valid()).isTrue();
        assertThat(result.line1()).isEqualTo("123 MAIN ST");
        assertThat(result.city()).isEqualTo("SPRINGFIELD");
        assertThat(result.state()).isEqualTo("IL");
        assertThat(result.postalCode()).isEqualTo("62704");
        assertThat(result.country()).isEqualTo("US");
        assertThat(result.normalizedAddress()).isEqualTo("123 MAIN ST, SPRINGFIELD, IL, 62704, US");
    }

    @Test
    void degradesGracefullyOnUnparseableAddress() {
        AddressNormalizer.Normalized result = normalizer.normalize("not a real address");

        assertThat(result.valid()).isFalse();
        assertThat(result.normalizedAddress()).isEqualTo("NOT A REAL ADDRESS");
    }

    @Test
    void handlesBlankInput() {
        AddressNormalizer.Normalized result = normalizer.normalize("  ");

        assertThat(result.valid()).isFalse();
        assertThat(result.normalizedAddress()).isEmpty();
    }
}
