package main

import (
	"context"
	"log"
	"os/signal"
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

	publisher := outbox.NewKafkaPublisher(cfg.KafkaBrokers)
	defer func() { _ = publisher.Close() }()

	relay := outbox.NewRelay(db, publisher, time.Second, logger.New("outbox-relay"))
	relay.Start(ctx)
	return nil
}
