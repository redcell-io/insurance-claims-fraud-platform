package io.redcell.policylookup.grpc;

import io.micrometer.prometheusmetrics.PrometheusConfig;
import io.micrometer.prometheusmetrics.PrometheusMeterRegistry;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * No Actuator autoconfiguration on this service (see GrpcServerLifecycle's
 * doc comment on why it stays non-web), so the PrometheusMeterRegistry bean
 * that Actuator would normally wire up has to be declared by hand.
 */
@Configuration
public class MetricsConfig {

    @Bean
    public PrometheusMeterRegistry prometheusMeterRegistry() {
        return new PrometheusMeterRegistry(PrometheusConfig.DEFAULT);
    }
}
