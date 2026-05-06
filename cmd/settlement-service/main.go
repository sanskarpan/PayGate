package main

import (
	"context"
	"log"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/config"
	"github.com/sanskarpan/PayGate/internal/common/logger"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/settlement"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config.FromEnv()
	l := logger.New("settlement-service")

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

	ledgerRepo := ledger.NewRepository(db)
	ledgerSvc := ledger.NewService(ledgerRepo)
	settlementRepo := settlement.NewPostgresRepository(db, ledgerSvc)
	settlementSvc := settlement.NewService(settlementRepo)

	l.Info("settlement-service started, running initial batch")
	runBatchAllMerchants(ctx, db, settlementSvc, l)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			runBatchAllMerchants(ctx, db, settlementSvc, l)
		}
	}
}

// runBatchAllMerchants queries all active merchants and runs a settlement batch
// covering all captured, non-settled payments up to now.
func runBatchAllMerchants(ctx context.Context, db *pgxpool.Pool, svc *settlement.Service, l *slog.Logger) {
	rows, err := db.Query(ctx, `SELECT id FROM paygate_merchants.merchants WHERE status = 'active'`)
	if err != nil {
		l.Error("list merchants for settlement", "error", err)
		return
	}
	defer rows.Close()

	var merchantIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			l.Error("scan merchant id", "error", err)
			continue
		}
		merchantIDs = append(merchantIDs, id)
	}
	rows.Close()

	periodStart := time.Unix(0, 0)
	periodEnd := time.Now().UTC()

	for _, merchantID := range merchantIDs {
		sttl, err := svc.RunBatch(ctx, merchantID, periodStart, periodEnd)
		if err == settlement.ErrNoEligiblePayments {
			l.Info("no eligible payments for settlement", "merchant_id", merchantID)
			continue
		}
		if err != nil {
			l.Error("settlement batch failed", "merchant_id", merchantID, "error", err)
			continue
		}
		l.Info("settlement batch completed",
			"merchant_id", merchantID,
			"settlement_id", sttl.ID,
			"net_amount", sttl.NetAmount,
			"payment_count", sttl.PaymentCount,
		)
	}
}
