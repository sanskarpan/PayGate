package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/config"
	"github.com/sanskarpan/PayGate/internal/common/logger"
	"github.com/sanskarpan/PayGate/internal/outbox"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config.FromEnv()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	publisher := outbox.NewKafkaPublisherWithTimeouts(cfg.KafkaBrokers, envDurationMillis("KAFKA_PUBLISH_TIMEOUT_MS", 5000), envDurationMillis("KAFKA_IO_TIMEOUT_MS", 5000))
	defer func() { _ = publisher.Close() }()

	relay := outbox.NewRelay(db, publisher, time.Second, logger.New("outbox-relay"))
	relay.Start(ctx)
	return nil
}

func envDurationMillis(name string, defaultMs int) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return time.Duration(defaultMs) * time.Millisecond
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return time.Duration(defaultMs) * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}
