package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// KafkaConsumer is the minimal interface the Consumer needs from a Kafka driver.
// Real implementations wrap segmentio/kafka-go; test implementations can be
// substituted without a live broker.
type KafkaConsumer interface {
	Subscribe(ctx context.Context, topics []string, handler func(topic, key string, payload []byte) error) error
}

// topics are the Kafka topics the Consumer subscribes to.
var topics = []string{
	"paygate.orders",
	"paygate.payments",
	"paygate.refunds",
	"paygate.settlements",
	"paygate.internal",
}

// Consumer subscribes to Kafka topics and dispatches each outbox envelope to
// the webhook Service for delivery.
type Consumer struct {
	svc           *Service
	kafkaConsumer KafkaConsumer
}

// NewConsumer creates a Consumer that reads from the given KafkaConsumer and
// delivers events through svc.
func NewConsumer(svc *Service, kc KafkaConsumer) *Consumer {
	return &Consumer{svc: svc, kafkaConsumer: kc}
}

// Start subscribes to all paygate Kafka topics and blocks until ctx is
// cancelled. The underlying KafkaConsumer.Subscribe call drives the message
// loop; Start returns whatever error Subscribe returns (including nil on clean
// shutdown).
func (c *Consumer) Start(ctx context.Context) error {
	return c.kafkaConsumer.Subscribe(ctx, topics, func(topic, key string, payload []byte) error {
		return c.HandleMessage(ctx, topic, key, payload)
	})
}

// outboxEnvelope matches the JSON structure written by outbox.Relay.PublishBatch.
type outboxEnvelope struct {
	ID            string          `json:"id"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	EventType     string          `json:"event_type"`
	MerchantID    string          `json:"merchant_id"`
	Payload       json.RawMessage `json:"payload"`
	CreatedAt     int64           `json:"created_at"`
	SchemaVersion string          `json:"schema_version"`
}

// HandleMessage parses an outbox envelope from payload and calls
// svc.DeliverEvent. It is exported so tests can call it directly without
// needing a real Kafka broker.
func (c *Consumer) HandleMessage(ctx context.Context, topic, key string, payload []byte) error {
	var env outboxEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return fmt.Errorf("webhook consumer: unmarshal outbox envelope from topic %s: %w", topic, err)
	}

	if env.ID == "" {
		return fmt.Errorf("webhook consumer: missing id in outbox envelope (topic=%s key=%s)", topic, key)
	}
	if env.MerchantID == "" {
		return fmt.Errorf("webhook consumer: missing merchant_id in outbox envelope (id=%s)", env.ID)
	}
	if env.EventType == "" {
		return fmt.Errorf("webhook consumer: missing event_type in outbox envelope (id=%s)", env.ID)
	}

	// Unmarshal the inner payload into a generic map so DeliverEvent can
	// re-serialise it when building the HTTP request body.
	var innerPayload map[string]any
	if len(env.Payload) > 0 {
		if err := json.Unmarshal(env.Payload, &innerPayload); err != nil {
			return fmt.Errorf("webhook consumer: unmarshal inner payload (id=%s): %w", env.ID, err)
		}
	}

	// Enrich the payload with envelope metadata so subscribers receive the
	// full context without needing to re-query the outbox.
	if innerPayload == nil {
		innerPayload = make(map[string]any)
	}
	innerPayload["event_id"] = env.ID
	innerPayload["event_type"] = env.EventType
	innerPayload["aggregate_type"] = env.AggregateType
	innerPayload["aggregate_id"] = env.AggregateID
	innerPayload["created_at"] = time.Unix(env.CreatedAt, 0).UTC().Format(time.RFC3339)

	if err := c.svc.DeliverEvent(ctx, env.ID, env.MerchantID, env.EventType, innerPayload); err != nil {
		return fmt.Errorf("webhook consumer: deliver event (id=%s): %w", env.ID, err)
	}
	return nil
}
