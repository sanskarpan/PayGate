package main

import (
	"context"
	"log"
	"log/slog"
	"os/signal"
	"sync"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	kafka "github.com/segmentio/kafka-go"
	"github.com/sanskarpan/PayGate/internal/common/config"
	"github.com/sanskarpan/PayGate/internal/common/logger"
	wh "github.com/sanskarpan/PayGate/internal/webhook"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config.FromEnv()
	l := logger.New("webhook-service")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		return err
	}

	var redisOpts []wh.Option
	rc := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rc.Ping(ctx).Err(); err != nil {
		l.Warn("redis unavailable, continuing without dedup cache", "error", err)
	} else {
		defer func() { _ = rc.Close() }()
		redisOpts = append(redisOpts, wh.WithRedis(rc))
	}

	webhookRepo := wh.NewPostgresRepository(db)
	webhookSvc := wh.NewService(webhookRepo, redisOpts...)
	consumer := wh.NewConsumer(webhookSvc, &kafkaReader{brokers: cfg.KafkaBrokers, logger: l})

	l.Info("webhook-service started, subscribing to kafka topics")
	return consumer.Start(ctx)
}

// kafkaReader implements wh.KafkaConsumer using segmentio/kafka-go.
// It starts one goroutine per topic and blocks until ctx is cancelled.
type kafkaReader struct {
	brokers []string
	logger  *slog.Logger
}

func (r *kafkaReader) Subscribe(ctx context.Context, topics []string, handler func(topic, key string, payload []byte) error) error {
	var wg sync.WaitGroup
	for _, topic := range topics {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			reader := kafka.NewReader(kafka.ReaderConfig{
				Brokers:  r.brokers,
				Topic:    t,
				GroupID:  "webhook-service",
				MinBytes: 1,
				MaxBytes: 1 << 20, // 1 MiB
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
