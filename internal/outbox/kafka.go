package outbox

import (
	"context"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaPublisher struct {
	brokers []string
	mu      sync.Mutex
	writers map[string]*kafka.Writer
}

func NewKafkaPublisher(brokers []string) *KafkaPublisher {
	return &KafkaPublisher{
		brokers: brokers,
		writers: map[string]*kafka.Writer{},
	}
}

func (p *KafkaPublisher) Publish(ctx context.Context, topic string, key string, payload []byte) error {
	writer := p.writer(topic)
	return writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: payload,
		Time:  time.Now().UTC(),
	})
}

func (p *KafkaPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var closeErr error
	for topic, writer := range p.writers {
		if err := writer.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		delete(p.writers, topic)
	}
	return closeErr
}

func (p *KafkaPublisher) writer(topic string) *kafka.Writer {
	p.mu.Lock()
	defer p.mu.Unlock()
	if writer, ok := p.writers[topic]; ok {
		return writer
	}
	writer := &kafka.Writer{
		Addr:         kafka.TCP(p.brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}
	p.writers[topic] = writer
	return writer
}
