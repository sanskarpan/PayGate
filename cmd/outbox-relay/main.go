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
	"github.com/sanskarpan/PayGate/internal/eventschema"
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

	schemaSvc := eventschema.NewService(eventschema.NewPostgresRepository(db), logger.New("event-schema"))
	if err := schemaSvc.BootstrapFromFixtures(ctx, "schemas/events", "platform"); err != nil {
		return err
	}
	relay := outbox.NewRelay(db, publisher, time.Second, logger.New("outbox-relay")).WithSchemaVersionResolver(schemaSvc)
	relay.Start(ctx)
	return nil
}
