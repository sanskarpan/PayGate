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
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/payout"
	"github.com/sanskarpan/PayGate/internal/saga"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config.FromEnv()
	l := logger.New("saga-orchestrator")

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

	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	sagaSvc := saga.NewService(saga.NewPostgresRepository(db), l)

	payoutSvc := payout.NewService(payout.NewPostgresRepository(db, ledgerSvc), l)
	payoutSvc.SetLedgerService(ledgerSvc)
	payoutSvc.EnableSagaOrchestration(sagaSvc)
	payoutSvc.RegisterSagaHandlers(sagaSvc)

	l.Info("saga-orchestrator started")
	saga.NewWorker(sagaSvc, time.Second, l).Start(ctx)
	return nil
}
