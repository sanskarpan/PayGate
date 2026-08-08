package webhook

import (
	"context"
	"errors"
	"log/slog"
	"time"

	kafka "github.com/segmentio/kafka-go"
)

// topicProvisionTimeout bounds the best-effort topic creation performed before
// the consumer group is joined.
const topicProvisionTimeout = 15 * time.Second

// KafkaReader implements KafkaConsumer using segmentio/kafka-go.
type KafkaReader struct {
	brokers []string
	groupID string
	logger  *slog.Logger
}

func NewKafkaReader(brokers []string, groupID string, logger *slog.Logger) *KafkaReader {
	if logger == nil {
		logger = slog.Default()
	}
	if groupID == "" {
		groupID = "webhook-service"
	}
	return &KafkaReader{brokers: brokers, groupID: groupID, logger: logger}
}

// Subscribe consumes every topic through a SINGLE consumer-group reader.
//
// Historically this created one kafka.Reader per topic, all sharing one
// GroupID. Members of a consumer group must agree on the topic set: with
// heterogeneous subscriptions the group can stabilise with zero partitions
// assigned to every member, which silently stops all webhook delivery. That is
// exactly what happens on a cold Kafka cluster, where the topics do not exist
// yet when the process starts.
//
// Using GroupTopics keeps a single membership covering all topics. Topics are
// also provisioned up front and partition changes are watched, so a broker that
// gains the topics later triggers a rebalance instead of leaving the reader idle.
func (r *KafkaReader) Subscribe(ctx context.Context, topics []string, handler func(topic, key string, payload []byte) error) error {
	if len(topics) == 0 {
		return nil
	}

	r.ensureTopics(ctx, topics)

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:               r.brokers,
		GroupTopics:           topics,
		GroupID:               r.groupID,
		MinBytes:              1,
		MaxBytes:              1 << 20,
		StartOffset:           kafka.FirstOffset,
		WatchPartitionChanges: true,
	})
	defer func() { _ = reader.Close() }()

	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			r.logger.Error("kafka read error", "error", err)
			continue
		}
		if err := handler(msg.Topic, string(msg.Key), msg.Value); err != nil {
			r.logger.Error("webhook consumer handler error", "topic", msg.Topic, "error", err)
		}
	}
}

// ensureTopics creates the subscribed topics if they do not already exist.
// Failures are logged and ignored: the broker may disallow topic creation, in
// which case the reader still works as long as the topics are provisioned out
// of band.
func (r *KafkaReader) ensureTopics(ctx context.Context, topics []string) {
	if len(r.brokers) == 0 {
		return
	}

	configs := make([]kafka.TopicConfig, 0, len(topics))
	for _, topic := range topics {
		configs = append(configs, kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1})
	}

	provisionCtx, cancel := context.WithTimeout(ctx, topicProvisionTimeout)
	defer cancel()

	client := &kafka.Client{Addr: kafka.TCP(r.brokers...), Timeout: topicProvisionTimeout}
	resp, err := client.CreateTopics(provisionCtx, &kafka.CreateTopicsRequest{Topics: configs})
	if err != nil {
		r.logger.Warn("could not provision webhook topics", "error", err)
		return
	}
	for topic, topicErr := range resp.Errors {
		if topicErr != nil && !errors.Is(topicErr, kafka.TopicAlreadyExists) {
			r.logger.Warn("could not provision webhook topic", "topic", topic, "error", topicErr)
		}
	}
}
