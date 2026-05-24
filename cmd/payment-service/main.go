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
	"github.com/sanskarpan/PayGate/internal/gateway"
	"github.com/sanskarpan/PayGate/internal/idempotency"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
	"github.com/sanskarpan/PayGate/internal/tokenization"
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
	orderSvc := order.NewService(order.NewPostgresRepository(db))
	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	cardTokenSvc := tokenization.NewService(tokenization.NewPostgresRepository(db))
	paymentSvc := payment.NewService(payment.NewPostgresRepository(db, ledgerSvc, orderSvc), gateway.NewSimulator(), payment.WithCardTokenAuthorizer(cardTokenSvc))
	paymentHandler := payment.NewHandler(paymentSvc, payment.WithCapabilityChecker(merchantSvc))
	cardTokenHandler := tokenization.NewHandler(cardTokenSvc)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", httpx.Healthz)
	protected := func(scope merchant.APIKeyScope, next http.Handler) http.Handler {
		return authMw.RequireScope(scope, idemMw.Wrap(next))
	}
	paymentHandler.RegisterRoutes(mux)
	paymentHandler.RegisterRoutesWithAuth(mux, protected)
	cardTokenHandler.RegisterRoutesWithAuth(mux, protected)

	port := os.Getenv("PAYMENT_SERVICE_PORT")
	if port == "" {
		port = "8092"
	}
	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	return srv.ListenAndServe()
}
