package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
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
	consumer := wh.NewConsumer(webhookSvc, wh.NewKafkaReader(cfg.KafkaBrokers, "webhook-service", l))

	l.Info("webhook-service started, subscribing to kafka topics")
	return consumer.Start(ctx)
}
