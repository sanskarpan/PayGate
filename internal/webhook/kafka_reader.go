package webhook

import (
	"context"
	"log/slog"
	"sync"

	kafka "github.com/segmentio/kafka-go"
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

func (r *KafkaReader) Subscribe(ctx context.Context, topics []string, handler func(topic, key string, payload []byte) error) error {
	var wg sync.WaitGroup
	for _, topic := range topics {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()

			reader := kafka.NewReader(kafka.ReaderConfig{
				Brokers:     r.brokers,
				Topic:       t,
				GroupID:     r.groupID,
				MinBytes:    1,
				MaxBytes:    1 << 20,
				StartOffset: kafka.FirstOffset,
			})
			defer func() { _ = reader.Close() }()

			for {
				msg, err := reader.ReadMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					r.logger.Error("kafka read error", "topic", t, "error", err)
					continue
				}
				if err := handler(t, string(msg.Key), msg.Value); err != nil {
					r.logger.Error("webhook consumer handler error", "topic", t, "error", err)
				}
			}
		}(topic)
	}
	wg.Wait()
	return nil
}
