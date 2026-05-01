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
	"github.com/redis/go-redis/v9"
	"github.com/sanskarpan/PayGate/internal/auth"
	"github.com/sanskarpan/PayGate/internal/common/config"
	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/common/logger"
	"github.com/sanskarpan/PayGate/internal/common/middleware"
	"github.com/sanskarpan/PayGate/internal/common/telemetry"
	"github.com/sanskarpan/PayGate/internal/gateway"
	"github.com/sanskarpan/PayGate/internal/idempotency"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
	"github.com/sanskarpan/PayGate/internal/refund"
	"github.com/sanskarpan/PayGate/internal/webhook"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config.FromEnv()
	if os.Getenv("APP_ENV") == "production" {
		if err := cfg.Validate(); err != nil {
			return err
		}
	}
	l := logger.New("api-gateway")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Init(ctx, "api-gateway")
	if err != nil {
		return err
	}
	defer func() { _ = shutdownTelemetry(context.Background()) }()

	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		return err
	}

	merchantRepo := merchant.NewPostgresRepository(db)
	merchantSvc := merchant.NewService(merchantRepo, merchant.WithSessionSecret(cfg.DashboardSessionSecret))
	merchantHandler := merchant.NewHandler(merchantSvc)
	authMw := auth.NewMiddleware(merchantSvc)
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		l.Warn("redis unavailable, falling back to db-only idempotency", "error", err)
		redisClient = nil
	} else {
		defer func() { _ = redisClient.Close() }()
	}
	idemMw := idempotency.NewMiddleware(idempotency.NewStore(db, redisClient))

	orderRepo := order.NewPostgresRepository(db)
	orderSvc := order.NewService(orderRepo)
	orderHandler := order.NewHandler(orderSvc)

	ledgerRepo := ledger.NewRepository(db)
	ledgerSvc := ledger.NewService(ledgerRepo)

	gatewayClient := gateway.NewSimulator()
	paymentRepo := payment.NewPostgresRepository(db, ledgerSvc, orderSvc)
	paymentSvc := payment.NewService(paymentRepo, gatewayClient)
	paymentHandler := payment.NewHandler(paymentSvc)
	checkoutHandler := gateway.NewCheckoutHandler(paymentSvc, orderSvc)

	refundRepo := refund.NewPostgresRepository(db, ledgerSvc)
	refundSvc := refund.NewService(refundRepo)
	refundHandler := refund.NewHandler(refundSvc)

	webhookRepo := webhook.NewPostgresRepository(db)
	webhookSvc := webhook.NewService(webhookRepo)
	webhookHandler := webhook.NewHandler(webhookSvc)

	go order.NewExpirySweeper(orderSvc, time.Minute, l).Start(ctx)
	go webhook.NewRetryWorker(webhookSvc, 30*time.Second, l).Start(ctx)
	go payment.NewSweeper(paymentSvc, 30*time.Second, l).Start(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", httpx.Healthz)
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{}
		ready := true

		if err := db.Ping(r.Context()); err != nil {
			checks["postgres"] = "unavailable"
			ready = false
		} else {
			checks["postgres"] = "ok"
		}

		if redisClient != nil {
			if err := redisClient.Ping(r.Context()).Err(); err != nil {
				checks["redis"] = "unavailable"
				// Redis is optional (idempotency falls back to Postgres) — not fatal.
			} else {
				checks["redis"] = "ok"
			}
		} else {
			checks["redis"] = "not_configured"
		}

		if !ready {
			httpx.WriteError(w, http.StatusServiceUnavailable, httpx.APIError{
				Code:        "SERVER_ERROR",
				Description: "one or more dependencies are not ready",
				Source:      "internal",
				Step:        "readiness_check",
				Reason:      "dependency_unavailable",
				Metadata:    map[string]any{"checks": checks},
			})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "checks": checks})
	})

	merchantHandler.RegisterRoutes(mux)
	protected := func(scope merchant.APIKeyScope, next http.Handler) http.Handler {
		return authMw.RequireScope(scope, idemMw.Wrap(next))
	}
	orderHandler.RegisterRoutesWithAuth(mux, protected)
	paymentHandler.RegisterRoutesWithAuth(mux, protected)
	refundHandler.RegisterRoutesWithAuth(mux, protected)
	webhookHandler.RegisterRoutesWithAuth(mux, protected)
	merchantHandler.RegisterProtectedRoutes(mux, protected)
	checkoutHandler.RegisterRoutes(mux)

	mux.Handle("GET /v1/merchants/me", authMw.RequireScope(merchant.APIKeyScopeRead, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := httpx.PrincipalFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "no authenticated principal in request context", Source: "auth", Step: "authentication", Reason: "missing_principal"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "merchant_auth_context", "merchant_id": p.MerchantID, "key_id": p.KeyID, "user_id": p.UserID, "email": p.Email, "role": p.Role, "scope": p.Scope, "auth_type": p.AuthType})
	})))

	mux.Handle("GET /v1/merchants/me/balance", authMw.RequireScope(merchant.APIKeyScopeRead, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := httpx.PrincipalFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
			return
		}
		accountCodes := []string{"CUSTOMER_RECEIVABLE", "MERCHANT_PAYABLE", "PLATFORM_FEE_REVENUE"}
		balances := make(map[string]int64, len(accountCodes))
		for _, code := range accountCodes {
			bal, err := ledgerSvc.GetBalance(r.Context(), p.MerchantID, code)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "could not retrieve balance"})
				return
			}
			balances[code] = bal
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"entity":      "merchant_balance",
			"merchant_id": p.MerchantID,
			"balances":    balances,
		})
	})))

	rateLimiter := middleware.NewRateLimiter(25, 25)
	handler := telemetry.WrapHTTP(httpx.NewRouter(middleware.Logging(l, rateLimiter.Middleware(mux))), "api-gateway")

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	l.Info("starting server", "addr", server.Addr)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
