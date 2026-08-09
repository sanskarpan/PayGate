package webhook

import (
	"context"
	"errors"
	"log/slog"
	"time"

	kafka "github.com/segmentio/kafka-go"
)

const (
	// topicProvisionTimeout bounds the best-effort topic creation performed
	// before the consumer group is joined.
	topicProvisionTimeout = 15 * time.Second

	// handlerMaxAttempts is how many times a fetched message is handed to the
	// handler before its offset is committed regardless.
	handlerMaxAttempts = 3
	// handlerRetryDelay is the base backoff between handler attempts.
	handlerRetryDelay = 500 * time.Millisecond
)

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
		// FetchMessage does not commit. ReadMessage would commit the offset
		// BEFORE handing the message over, so anything that failed before the
		// delivery attempt was persisted — a database blip, a malformed
		// envelope — was silently lost. Commit only once the handler has
		// succeeded.
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			r.logger.Error("kafka read error", "error", err)
			continue
		}

		if err := r.handleWithRetry(ctx, msg, handler); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// Retries are exhausted. Commit anyway: a permanently poisonous
			// message must not stall webhook delivery for every merchant on
			// the partition. Delivery failures are already recorded and
			// retried by the webhook service itself, so this only covers
			// envelopes that could never be processed.
			r.logger.Error("webhook consumer dropping message after retries",
				"topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset,
				"key", string(msg.Key), "error", err)
		}

		if err := reader.CommitMessages(ctx, msg); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			r.logger.Error("kafka commit error", "topic", msg.Topic, "offset", msg.Offset, "error", err)
		}
	}
}

// handleWithRetry invokes handler, retrying a bounded number of times so a
// transient failure does not drop the event.
func (r *KafkaReader) handleWithRetry(ctx context.Context, msg kafka.Message, handler func(topic, key string, payload []byte) error) error {
	var err error
	for attempt := 1; attempt <= handlerMaxAttempts; attempt++ {
		if err = handler(msg.Topic, string(msg.Key), msg.Value); err == nil {
			return nil
		}
		if attempt == handlerMaxAttempts {
			break
		}
		r.logger.Warn("webhook consumer handler error, retrying",
			"topic", msg.Topic, "offset", msg.Offset, "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(handlerRetryDelay * time.Duration(attempt)):
		}
	}
	return err
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
