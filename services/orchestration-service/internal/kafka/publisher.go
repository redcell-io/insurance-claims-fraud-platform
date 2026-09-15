// Package kafka publishes scored claim events to the claims.realtime
// topic (DESIGN.md §10). Thin slice: JSON-encoded messages, no
// Protobuf wire format or schema registry yet — DESIGN.md's target
// messaging convention ("Protobuf + schema registry") is deferred the
// same way KMS-derived keys, Postgres-backed config, etc. were
// deferred elsewhere in this repo. See DECISIONS.md.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	kafkago "github.com/segmentio/kafka-go"
)

// TopicClaimsRealtime is DESIGN.md §10's scored-claims topic,
// partitioned by tenant_id (preferred over per-tenant topics, to avoid
// topic sprawl).
const TopicClaimsRealtime = "claims.realtime"

// ScoredClaimEvent is the JSON shape published to TopicClaimsRealtime.
// Deliberately mirrors ProcessClaimResponse (orchestration.proto) plus
// tenant_id, which the response itself doesn't carry (ClaimsGateway
// already knows it) but downstream consumers need for partitioning and
// per-tenant routing.
type ScoredClaimEvent struct {
	ClaimID       string  `json:"claim_id"`
	CorrelationID string  `json:"correlation_id"`
	TenantID      string  `json:"tenant_id"`
	Status        string  `json:"status"`
	FraudScore    float64 `json:"fraud_score"`
	ModelVersion  string  `json:"model_version"`
}

// Publisher is the narrow interface Executor needs, so it can be
// tested without a live broker connection (mirrors AddressNormalizer/
// Scorer/etc. in dag/executor.go).
type Publisher interface {
	Publish(ctx context.Context, event ScoredClaimEvent) error
}

// KafkaPublisher is the real Publisher, backed by a kafka-go Writer.
// The Writer dials lazily on first write, not at construction — a
// broker that isn't up yet doesn't block orchestration-service's
// startup, consistent with the publish DAG node's on_failure: skip
// (a Kafka outage degrades a request, it doesn't take the service
// down).
type KafkaPublisher struct {
	writer *kafkago.Writer
}

// NewPublisher builds a KafkaPublisher targeting a single broker
// address (host:port). Partitions by event.TenantID — see
// TopicClaimsRealtime's doc comment.
func NewPublisher(brokerAddr string) *KafkaPublisher {
	return &KafkaPublisher{
		writer: &kafkago.Writer{
			Addr:                   kafkago.TCP(brokerAddr),
			Topic:                  TopicClaimsRealtime,
			Balancer:               &kafkago.Hash{},
			RequiredAcks:           kafkago.RequireOne,
			AllowAutoTopicCreation: true, // thin slice: no separate topic-provisioning step yet
		},
	}
}

func (p *KafkaPublisher) Publish(ctx context.Context, event ScoredClaimEvent) error {
	value, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal scored claim event: %w", err)
	}
	if err := p.writer.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(event.TenantID),
		Value: value,
	}); err != nil {
		return fmt.Errorf("write to %s: %w", TopicClaimsRealtime, err)
	}
	return nil
}

// Close flushes and closes the underlying writer. Call on service
// shutdown (defer, same pattern as the gRPC client connections in
// cmd/orchestration/main.go).
func (p *KafkaPublisher) Close() error {
	return p.writer.Close()
}
