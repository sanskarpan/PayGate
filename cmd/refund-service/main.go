package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/auth"
	"github.com/sanskarpan/PayGate/internal/common/config"
	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/idempotency"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/refund"
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
	if err := db.Ping(ctx); err != nil {
		return err
	}

	merchantSvc := merchant.NewService(merchant.NewPostgresRepository(db), merchant.WithSessionSecret(cfg.DashboardSessionSecret))
	authMw := auth.NewMiddlewareWithTrustedProxyCIDRs(merchantSvc, cfg.TrustedProxyCIDRs)
	idemMw := idempotency.NewMiddleware(idempotency.NewStore(db, nil))
	refundHandler := refund.NewHandler(refund.NewService(refund.NewPostgresRepository(db, ledger.NewService(ledger.NewRepository(db)))), merchantSvc)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", httpx.Healthz)
	protected := func(scope merchant.APIKeyScope, next http.Handler) http.Handler {
		return authMw.RequireScope(scope, idemMw.Wrap(next))
	}
	refundHandler.RegisterRoutesWithAuth(mux, protected)

	port := os.Getenv("REFUND_SERVICE_PORT")
	if port == "" {
		port = "8093"
	}
	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	return srv.ListenAndServe()
}
