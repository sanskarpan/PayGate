package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/sanskarpan/PayGate/internal/audit"
	"github.com/sanskarpan/PayGate/internal/auth"
	"github.com/sanskarpan/PayGate/internal/common/config"
	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/common/logger"
	appmetrics "github.com/sanskarpan/PayGate/internal/common/metrics"
	"github.com/sanskarpan/PayGate/internal/common/middleware"
	"github.com/sanskarpan/PayGate/internal/common/telemetry"
	"github.com/sanskarpan/PayGate/internal/dispute"
	"github.com/sanskarpan/PayGate/internal/eventschema"
	"github.com/sanskarpan/PayGate/internal/gateway"
	"github.com/sanskarpan/PayGate/internal/idempotency"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/outbox"
	"github.com/sanskarpan/PayGate/internal/payment"
	"github.com/sanskarpan/PayGate/internal/payout"
	"github.com/sanskarpan/PayGate/internal/recon"
	"github.com/sanskarpan/PayGate/internal/refund"
	"github.com/sanskarpan/PayGate/internal/risk"
	"github.com/sanskarpan/PayGate/internal/saga"
	"github.com/sanskarpan/PayGate/internal/settlement"
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
	authMw := auth.NewMiddlewareWithTrustedProxyCIDRs(merchantSvc, cfg.TrustedProxyCIDRs)
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
	ledgerHoldHandler := ledger.NewHoldHandler(ledgerSvc)

	riskRepo := risk.NewPostgresRepository(db)
	riskSvc := risk.NewService(riskRepo, l)
	riskHandler := risk.NewHandler(riskSvc)

	scenarioStore := gateway.NewScenarioStore(db)
	methodStore := gateway.NewMethodConfigStore(db)
	gatewayClient := gateway.NewSimulatorWithStores(scenarioStore, methodStore)
	gatewayAdminHandler := gateway.NewAdminHandler(scenarioStore)
	methodHandler := gateway.NewMethodHandler(methodStore)
	paymentRepo := payment.NewPostgresRepository(db, ledgerSvc, orderSvc)
	paymentSvc := payment.NewService(paymentRepo, gatewayClient)
	paymentHandler := payment.NewHandler(paymentSvc, payment.WithRiskEvaluator(&riskAdapter{svc: riskSvc}))
	checkoutHandler := gateway.NewCheckoutHandler(paymentSvc, orderSvc)

	refundRepo := refund.NewPostgresRepository(db, ledgerSvc)
	refundSvc := refund.NewService(refundRepo)
	refundHandler := refund.NewHandler(refundSvc)

	settlementRepo := settlement.NewPostgresRepository(db, ledgerSvc)
	settlementSvc := settlement.NewService(settlementRepo)
	settlementHandler := settlement.NewHandler(settlementSvc)

	webhookRepo := webhook.NewPostgresRepository(db)
	webhookSvc := webhook.NewService(webhookRepo)
	webhookHandler := webhook.NewHandler(webhookSvc)
	webhookConsumer := webhook.NewConsumer(webhookSvc, webhook.NewKafkaReader(cfg.KafkaBrokers, "webhook-service", l))

	eventSchemaSvc := eventschema.NewService(eventschema.NewPostgresRepository(db), l)
	if err := eventSchemaSvc.BootstrapFromFixtures(ctx, "schemas/events", "platform"); err != nil {
		return fmt.Errorf("bootstrap event schemas: %w", err)
	}
	eventSchemaHandler := eventschema.NewHandler(eventSchemaSvc)
	go eventschema.NewAlertChecker(eventSchemaSvc, time.Minute, l).Start(ctx)

	var outboxPublisher outbox.Publisher = outbox.NewKafkaPublisher(cfg.KafkaBrokers)
	if os.Getenv("APP_ENV") != "production" {
		outboxPublisher = outbox.NewFallbackPublisher(outboxPublisher, outbox.NewLocalPublisher(func(topic, key string, payload []byte) error {
			return webhookConsumer.HandleMessage(ctx, topic, key, payload)
		}), l)
	}
	go outbox.NewRelay(db, outboxPublisher, time.Second, l).WithSchemaVersionResolver(eventSchemaSvc).Start(ctx)
	go func() {
		if err := webhookConsumer.Start(ctx); err != nil && ctx.Err() == nil {
			l.Error("webhook consumer stopped", "error", err)
		}
	}()

	auditRepo := audit.NewPostgresRepository(db)
	auditSvc := audit.NewService(auditRepo, l)
	auditHandler := audit.NewHandler(auditSvc)

	go order.NewExpirySweeper(orderSvc, time.Minute, l).Start(ctx)
	go webhook.NewRetryWorker(webhookSvc, 30*time.Second, l).Start(ctx)
	go payment.NewSweeper(paymentSvc, 30*time.Second, l).Start(ctx)
	go ledger.NewHoldSweeper(ledgerSvc, time.Minute, l).Start(ctx)
	reconWorker := recon.NewWorker(db, l)
	go reconWorker.Start(ctx)
	reconHandler := recon.NewHandler(reconWorker)
	sagaRepo := saga.NewPostgresRepository(db)
	sagaSvc := saga.NewService(sagaRepo, l)
	sagaHandler := saga.NewHandler(sagaSvc)
	if cfg.SagaWorkerEnabled {
		go saga.NewWorker(sagaSvc, time.Second, l).Start(ctx)
	}

	schemaRepo := eventschema.NewPostgresRepository(db)
	schemaSvc := eventschema.NewService(schemaRepo, nil)
	schemaHandler := eventschema.NewHandler(schemaSvc)

	holdHandler := ledger.NewHoldHandler(ledgerSvc)

	// Dispute management
	disputeRepo := dispute.NewPostgresRepository(db)
	disputeSvc := dispute.NewService(disputeRepo)
	disputeHandler := dispute.NewHandler(disputeSvc)

	// Payout workflow
	payoutRepo := payout.NewPostgresRepository(db, ledgerSvc)
	payoutSvc := payout.NewService(payoutRepo, l)
	payoutSvc.SetLedgerService(ledgerSvc)
	payoutSvc.SetRailCallbackSecret(cfg.PayoutRailSecret)
	payoutSvc.EnableSagaOrchestration(sagaSvc)
	payoutSvc.RegisterSagaHandlers(sagaSvc)
	payoutHandler := payout.NewHandler(payoutSvc, settlementSvc, ledgerSvc)

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

		// Outbox lag metric: count of unpublished entries.
		var unpublished int64
		if err := db.QueryRow(r.Context(), `SELECT COUNT(*) FROM public.outbox WHERE published_at IS NULL`).Scan(&unpublished); err == nil {
			checks["outbox_unpublished"] = fmt.Sprintf("%d", unpublished)
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
		return authMw.RequireScope(scope, idemMw.Wrap(auditSvc.Middleware(next)))
	}
	orderHandler.RegisterRoutesWithAuth(mux, protected)
	paymentHandler.RegisterRoutesWithAuth(mux, protected)
	refundHandler.RegisterRoutesWithAuth(mux, protected)
	ledgerHoldHandler.RegisterRoutesWithAuth(mux, protected)
	settlementHandler.RegisterRoutesWithAuth(mux, protected)
	webhookHandler.RegisterRoutesWithAuth(mux, protected)
	eventSchemaHandler.RegisterRoutesWithAuth(mux, protected)
	merchantHandler.RegisterProtectedRoutes(mux, protected)
	checkoutHandler.RegisterRoutes(mux)
	auditHandler.RegisterRoutesWithAuth(mux, func(next http.Handler) http.Handler {
		return authMw.RequireScope(merchant.APIKeyScopeRead, next)
	})
	riskHandler.RegisterRoutesWithAuth(mux, func(scope string, next http.Handler) http.Handler {
		s := merchant.APIKeyScope(scope)
		return authMw.RequireScope(s, next)
	})
	reconHandler.RegisterRoutesWithAuth(mux, func(next http.Handler) http.Handler {
		return authMw.RequireScope(merchant.APIKeyScopeRead, next)
	})
	registerAdvancedPlatformRoutes(mux, protected, sagaHandler, schemaHandler, holdHandler)

	// Disputes
	disputeHandler.RegisterRoutesWithAuth(mux, protected)

	// Payouts
	payoutHandler.RegisterRoutesWithAuth(mux, protected)
	payoutHandler.RegisterPublicRoutes(mux)

	// Gateway simulator control panel (no merchant auth - admin endpoint)
	gatewayAdminHandler.RegisterRoutesWithAuth(mux, func(next http.Handler) http.Handler {
		return authMw.RequireScope(merchant.APIKeyScopeAdmin, next)
	})
	methodHandler.RegisterRoutesWithAuth(mux, func(next http.Handler) http.Handler {
		return authMw.RequireScope(merchant.APIKeyScopeAdmin, next)
	})

	// Prometheus metrics
	mux.Handle("GET /metrics", promhttp.Handler())

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
	dashboardOrigin := cfg.DashboardOrigin
	if dashboardOrigin == "" {
		dashboardOrigin = "http://localhost:3001"
	}
	handler := telemetry.WrapHTTP(
		httpx.NewRouter(
			middleware.CORS(dashboardOrigin)(
				appmetrics.MetricsMiddleware(
					middleware.Logging(l,
						middleware.MaxBody(middleware.MaxBodyBytes,
							rateLimiter.Middleware(mux),
						),
					),
				),
			),
		),
		"api-gateway",
	)

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

// riskAdapter bridges the risk.Service to the payment.RiskEvaluator interface.
type riskAdapter struct {
	svc *risk.Service
}

func (a *riskAdapter) EvaluateAuthorize(ctx context.Context, merchantID, paymentID string, amount int64, ipAddress string) (string, error) {
	ev, err := a.svc.EvaluatePayment(ctx, risk.EvalInput{
		MerchantID: merchantID,
		PaymentID:  paymentID,
		Amount:     amount,
		Currency:   "INR",
		IPAddress:  ipAddress,
	})
	if err != nil {
		return string(risk.RiskActionAllow), err
	}
	return string(ev.Action), nil
}

func registerAdvancedPlatformRoutes(
	mux *http.ServeMux,
	wrap func(scope merchant.APIKeyScope, next http.Handler) http.Handler,
	sagaHandler *saga.Handler,
	schemaHandler *eventschema.Handler,
	holdHandler *ledger.HoldHandler,
) {
	advancedMux := http.NewServeMux()
	sagaHandler.RegisterRoutesWithAuth(advancedMux, wrap)
	schemaHandler.RegisterRoutesWithAuth(advancedMux, wrap)
	holdHandler.RegisterRoutesWithAuth(advancedMux, wrap)

	for _, pattern := range []string{
		"/v1/sagas",
		"/v1/sagas/",
		"/v1/event-schemas",
		"/v1/event-schemas/",
		"/v1/event-schema-rollouts/",
		"/v1/ledger/holds",
		"/v1/ledger/holds/",
	} {
		mux.Handle(pattern, advancedMux)
	}
}
