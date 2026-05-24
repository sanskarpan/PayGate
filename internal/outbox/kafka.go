package outbox

import (
	"context"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaPublisher struct {
	brokers        []string
	publishTimeout time.Duration
	ioTimeout      time.Duration
	mu             sync.Mutex
	writers        map[string]*kafka.Writer
}

func NewKafkaPublisher(brokers []string) *KafkaPublisher {
	return NewKafkaPublisherWithTimeouts(brokers, 5*time.Second, 5*time.Second)
}

func NewKafkaPublisherWithTimeouts(brokers []string, publishTimeout, ioTimeout time.Duration) *KafkaPublisher {
	if publishTimeout <= 0 {
		publishTimeout = 5 * time.Second
	}
	if ioTimeout <= 0 {
		ioTimeout = 5 * time.Second
	}
	return &KafkaPublisher{
		brokers:        brokers,
		publishTimeout: publishTimeout,
		ioTimeout:      ioTimeout,
		writers:        map[string]*kafka.Writer{},
	}
}

func (p *KafkaPublisher) Publish(ctx context.Context, topic string, key string, payload []byte) error {
	writer := p.writer(topic)
	writeCtx, cancel := context.WithTimeout(ctx, p.publishTimeout)
	defer cancel()

	return writer.WriteMessages(writeCtx, kafka.Message{
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
		MaxAttempts:  3,
		ReadTimeout:  p.ioTimeout,
		WriteTimeout: p.ioTimeout,
		Transport: &kafka.Transport{
			DialTimeout: p.ioTimeout,
			MetadataTTL: 30 * time.Second,
		},
	}
	p.writers[topic] = writer
	return writer
}
