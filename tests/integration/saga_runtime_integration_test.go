//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/auth"
	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/gateway"
	"github.com/sanskarpan/PayGate/internal/idempotency"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
	"github.com/sanskarpan/PayGate/internal/payout"
	"github.com/sanskarpan/PayGate/internal/saga"
	"github.com/sanskarpan/PayGate/internal/settlement"
)

func TestIntegrationSagaTimeoutCompensatesPayoutAndExposesRuntimeState(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	env := buildSagaRuntimeEnv(t, db, func(ctx context.Context, merchantID, payoutID, commandID string) (map[string]any, error) {
		time.Sleep(200 * time.Millisecond)
		return map[string]any{"payout_id": payoutID, "status": "late_success"}, nil
	})
	env.sagaSvc.SetLeaseTTLForTest(25 * time.Millisecond)
	go saga.NewWorker(env.sagaSvc, 10*time.Millisecond, nil).Start(context.Background())

	merchantID, authHeader, sttl := seedPayoutSettlement(t, ctx, env.merchantSvc, env.orderSvc, env.paymentSvc, env.settlementSvc)
	po, err := env.payoutRepo.CreateForSettlement(ctx, merchantID, sttl.ID, sttl.NetAmount, sttl.Currency)
	if err != nil {
		t.Fatalf("create payout: %v", err)
	}
	po, err = env.payoutRepo.Initiate(ctx, merchantID, po.ID)
	if err != nil {
		t.Fatalf("initiate payout: %v", err)
	}

	timeoutAt := time.Now().Add(40 * time.Millisecond)
	instance, err := env.sagaSvc.StartCommandSaga(ctx, saga.CreateCommandSagaInput{
		MerchantID: merchantID,
		SagaType:   "payout_execution",
		InputPayload: map[string]any{
			"payout_id": po.ID,
		},
		ContextPayload: map[string]any{
			"payout_id": po.ID,
		},
		TimeoutAt: &timeoutAt,
		InitialStep: saga.CreateStepInput{
			StepName:    "complete_payout_transfer",
			StepKind:    saga.StepKindCommand,
			CommandName: "payout.complete_transfer",
			InputPayload: map[string]any{
				"payout_id": po.ID,
			},
			MaxAttempts: 1,
		},
	})
	if err != nil {
		t.Fatalf("start saga: %v", err)
	}
	if _, err := env.payoutRepo.AttachSaga(ctx, merchantID, po.ID, instance.ID); err != nil {
		t.Fatalf("attach saga: %v", err)
	}

	waitFor(t, 5*time.Second, func() bool {
		current, err := env.payoutRepo.GetByID(ctx, merchantID, po.ID)
		if err != nil {
			return false
		}
		inst, err := env.sagaSvc.Get(ctx, merchantID, instance.ID)
		if err != nil {
			return false
		}
		return current.Status == payout.StateFailed && inst.Status == saga.StatusAborted
	})

	current, err := env.payoutRepo.GetByID(ctx, merchantID, po.ID)
	if err != nil {
		t.Fatalf("get compensated payout: %v", err)
	}
	if current.Status != payout.StateFailed {
		t.Fatalf("expected payout to be failed by compensation, got %s", current.Status)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sagas/"+instance.ID+"/dead-letters", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get dead letters: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var deadLetters map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &deadLetters); err != nil {
		t.Fatalf("decode dead letters: %v", err)
	}
	items := deadLetters["items"].([]any)
	if len(items) == 0 {
		t.Fatal("expected at least one dead letter")
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/sagas/"+instance.ID+"/actions", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get actions: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var actions map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &actions); err != nil {
		t.Fatalf("decode actions: %v", err)
	}
	actionItems := actions["items"].([]any)
	if len(actionItems) == 0 {
		t.Fatal("expected at least one operator action")
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/sagas/"+instance.ID+"/dispatches", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get dispatches: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var dispatches map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &dispatches); err != nil {
		t.Fatalf("decode dispatches: %v", err)
	}
	dispatchItems := dispatches["items"].([]any)
	if len(dispatchItems) == 0 {
		t.Fatal("expected at least one dispatch record")
	}
	firstDispatch := dispatchItems[0].(map[string]any)
	if firstDispatch["status"] != "nacked" {
		t.Fatalf("expected first dispatch to be nacked, got %#v", firstDispatch["status"])
	}
}

func TestIntegrationSagaOperatorAbortCompensatesPayout(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	env := buildSagaRuntimeEnv(t, db, func(ctx context.Context, merchantID, payoutID, commandID string) (map[string]any, error) {
		time.Sleep(300 * time.Millisecond)
		return map[string]any{"payout_id": payoutID, "status": "late_success"}, nil
	})
	env.sagaSvc.SetLeaseTTLForTest(50 * time.Millisecond)
	go saga.NewWorker(env.sagaSvc, 10*time.Millisecond, nil).Start(context.Background())

	merchantID, authHeader, sttl := seedPayoutSettlement(t, ctx, env.merchantSvc, env.orderSvc, env.paymentSvc, env.settlementSvc)
	po, err := env.payoutSvc.InitiatePayoutForSettlement(ctx, merchantID, sttl.ID, sttl.NetAmount, sttl.Currency)
	if err != nil {
		t.Fatalf("initiate payout saga: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		current, err := env.payoutRepo.GetByID(ctx, merchantID, po.ID)
		return err == nil && current.SagaID != ""
	})
	current, err := env.payoutRepo.GetByID(ctx, merchantID, po.ID)
	if err != nil {
		t.Fatalf("get payout before override: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sagas/"+current.SagaID+"/override", strings.NewReader(`{"action":"abort","reason":"manual operator stop"}`))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("override saga: expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}

	waitFor(t, 5*time.Second, func() bool {
		poNow, err := env.payoutRepo.GetByID(ctx, merchantID, po.ID)
		if err != nil {
			return false
		}
		inst, err := env.sagaSvc.Get(ctx, merchantID, current.SagaID)
		if err != nil {
			return false
		}
		return poNow.Status == payout.StateFailed && inst.Status == saga.StatusAborted
	})
}

func TestIntegrationSagaDuplicateCommandLeaseDoesNotDoublePostPayout(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	var executions atomic.Int32
	var env sagaRuntimeEnv
	env = buildSagaRuntimeEnv(t, db, func(ctx context.Context, merchantID, payoutID, commandID string) (map[string]any, error) {
		executions.Add(1)
		time.Sleep(90 * time.Millisecond)
		processingAt := time.Now().UTC()
		if _, _, err := env.payoutRepo.ApplyRailCallback(ctx, payout.RailCallback{
			EventID:    commandID + "_processing",
			PayoutID:   payoutID,
			MerchantID: merchantID,
			Status:     payout.RailStatusProcessing,
			OccurredAt: processingAt,
		}, "simulated"); err != nil {
			return nil, err
		}
		out, _, err := env.payoutRepo.ApplyRailCallback(ctx, payout.RailCallback{
			EventID:       commandID + "_completed",
			PayoutID:      payoutID,
			MerchantID:    merchantID,
			Status:        payout.RailStatusCompleted,
			RailReference: "BNK_DUP_SAFE",
			OccurredAt:    processingAt.Add(10 * time.Millisecond),
		}, "simulated")
		if err != nil {
			return nil, err
		}
		return map[string]any{"payout_id": out.ID, "status": out.Status, "bank_reference": out.BankReference}, nil
	})
	env.sagaSvc.SetLeaseTTLForTest(25 * time.Millisecond)
	go saga.NewWorker(env.sagaSvc, 10*time.Millisecond, nil).Start(context.Background())
	go saga.NewWorker(env.sagaSvc, 10*time.Millisecond, nil).Start(context.Background())

	merchantID, _, sttl := seedPayoutSettlement(t, ctx, env.merchantSvc, env.orderSvc, env.paymentSvc, env.settlementSvc)
	po, err := env.payoutSvc.InitiatePayoutForSettlement(ctx, merchantID, sttl.ID, sttl.NetAmount, sttl.Currency)
	if err != nil {
		t.Fatalf("initiate payout: %v", err)
	}

	waitFor(t, 5*time.Second, func() bool {
		current, err := env.payoutRepo.GetByID(ctx, merchantID, po.ID)
		if err != nil {
			return false
		}
		inst, err := env.sagaSvc.Get(ctx, merchantID, current.SagaID)
		if err != nil {
			return false
		}
		return current.Status == payout.StateCompleted && inst.Status == saga.StatusCompleted
	})
	if executions.Load() < 2 {
		t.Fatalf("expected duplicate handler execution under stale lease, got %d", executions.Load())
	}

	var ledgerEntries int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM paygate_ledger.ledger_entries WHERE source_id = $1`, po.ID).Scan(&ledgerEntries); err != nil {
		t.Fatalf("count payout ledger entries: %v", err)
	}
	if ledgerEntries != 2 {
		t.Fatalf("expected exactly 2 ledger entries for payout despite duplicate execution, got %d", ledgerEntries)
	}

	var callbackReceipts int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM paygate_payouts.payout_callback_receipts WHERE payout_id = $1`, po.ID).Scan(&callbackReceipts); err != nil {
		t.Fatalf("count callback receipts: %v", err)
	}
	if callbackReceipts != 2 {
		t.Fatalf("expected exactly 2 callback receipts despite duplicate execution, got %d", callbackReceipts)
	}
}

func TestIntegrationSagaWorkerRestartRetriesPayoutWithoutDuplicatePostings(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	var attempts atomic.Int32
	var env sagaRuntimeEnv
	env = buildSagaRuntimeEnv(t, db, func(ctx context.Context, merchantID, payoutID, commandID string) (map[string]any, error) {
		attempt := attempts.Add(1)
		if attempt == 1 {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		out, _, err := env.payoutRepo.ApplyRailCallback(ctx, payout.RailCallback{
			EventID:    commandID + "_processing",
			PayoutID:   payoutID,
			MerchantID: merchantID,
			Status:     payout.RailStatusProcessing,
			OccurredAt: time.Now().UTC(),
		}, "simulated")
		if err != nil {
			return nil, err
		}
		out, _, err = env.payoutRepo.ApplyRailCallback(ctx, payout.RailCallback{
			EventID:       commandID + "_completed",
			PayoutID:      payoutID,
			MerchantID:    merchantID,
			Status:        payout.RailStatusCompleted,
			RailReference: "BNK_RESTART_OK",
			OccurredAt:    time.Now().UTC().Add(25 * time.Millisecond),
		}, "simulated")
		if err != nil {
			return nil, err
		}
		return map[string]any{"payout_id": out.ID, "status": out.Status, "bank_reference": out.BankReference}, nil
	})
	env.sagaSvc.SetLeaseTTLForTest(25 * time.Millisecond)

	merchantID, _, sttl := seedPayoutSettlement(t, ctx, env.merchantSvc, env.orderSvc, env.paymentSvc, env.settlementSvc)
	po, err := env.payoutSvc.InitiatePayoutForSettlement(ctx, merchantID, sttl.ID, sttl.NetAmount, sttl.Currency)
	if err != nil {
		t.Fatalf("initiate payout: %v", err)
	}

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		saga.NewWorker(env.sagaSvc, 10*time.Millisecond, nil).Start(workerCtx)
	}()

	waitFor(t, 5*time.Second, func() bool {
		return attempts.Load() >= 1
	})
	cancelWorker()
	<-done

	restartCtx, cancelRestart := context.WithCancel(context.Background())
	defer cancelRestart()
	go saga.NewWorker(env.sagaSvc, 10*time.Millisecond, nil).Start(restartCtx)

	waitFor(t, 5*time.Second, func() bool {
		current, err := env.payoutRepo.GetByID(ctx, merchantID, po.ID)
		if err != nil {
			return false
		}
		inst, err := env.sagaSvc.Get(ctx, merchantID, current.SagaID)
		if err != nil {
			return false
		}
		return current.Status == payout.StateCompleted && current.BankReference == "BNK_RESTART_OK" && inst.Status == saga.StatusCompleted
	})

	if attempts.Load() < 2 {
		t.Fatalf("expected worker restart to cause a retry, got %d attempts", attempts.Load())
	}

	current, err := env.payoutRepo.GetByID(ctx, merchantID, po.ID)
	if err != nil {
		t.Fatalf("get payout after restart: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/sagas/"+current.SagaID+"/dispatches", nil)
	req.Header.Set("Authorization", basicAuthFromMerchant(t, env.merchantSvc, merchantID))
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get restart dispatches: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var dispatches map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &dispatches); err != nil {
		t.Fatalf("decode restart dispatches: %v", err)
	}
	dispatchItems := dispatches["items"].([]any)
	if len(dispatchItems) < 2 {
		t.Fatalf("expected at least 2 dispatch attempts, got %d", len(dispatchItems))
	}

	var ledgerEntries int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM paygate_ledger.ledger_entries WHERE source_id = $1`, po.ID).Scan(&ledgerEntries); err != nil {
		t.Fatalf("count payout ledger entries: %v", err)
	}
	if ledgerEntries != 2 {
		t.Fatalf("expected exactly 2 ledger entries after worker restart retry, got %d", ledgerEntries)
	}
}

type sagaRuntimeEnv struct {
	mux           *http.ServeMux
	merchantSvc   *merchant.Service
	orderSvc      *order.Service
	paymentSvc    *payment.Service
	settlementSvc *settlement.Service
	payoutSvc     *payout.Service
	payoutRepo    *payout.PostgresRepository
	sagaSvc       *saga.Service
}

func buildSagaRuntimeEnv(t *testing.T, db *pgxpool.Pool, transferFn func(context.Context, string, string, string) (map[string]any, error)) sagaRuntimeEnv {
	t.Helper()
	merchantRepo := merchant.NewPostgresRepository(db)
	merchantSvc := merchant.NewService(merchantRepo, merchant.WithSessionSecret("integration-dashboard-secret"))
	authMw := auth.NewMiddleware(merchantSvc)
	idemMw := idempotency.NewMiddleware(idempotency.NewStore(db, nil))

	orderSvc := order.NewService(order.NewPostgresRepository(db))
	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	paymentSvc := payment.NewService(payment.NewPostgresRepository(db, ledgerSvc, orderSvc), gateway.NewSimulator())
	settlementSvc := settlement.NewService(settlement.NewPostgresRepository(db, ledgerSvc))
	sagaSvc := saga.NewService(saga.NewPostgresRepository(db), nil)
	payoutRepo := payout.NewPostgresRepository(db, ledgerSvc)
	payoutSvc := payout.NewService(payoutRepo, nil)
	payoutSvc.SetLedgerService(ledgerSvc)
	payoutSvc.EnableSagaOrchestration(sagaSvc)
	payoutSvc.RegisterSagaHandlers(sagaSvc)
	payoutSvc.SetTransferExecutorForTest(transferFn)

	merchantHandler := merchant.NewHandler(merchantSvc)
	orderHandler := order.NewHandler(orderSvc)
	paymentHandler := payment.NewHandler(paymentSvc)
	settlementHandler := settlement.NewHandler(settlementSvc)
	payoutHandler := payout.NewHandler(payoutSvc, settlementSvc)
	sagaHandler := saga.NewHandler(sagaSvc)

	protected := func(scope merchant.APIKeyScope, next http.Handler) http.Handler {
		return authMw.RequireScope(scope, idemMw.Wrap(next))
	}

	mux := http.NewServeMux()
	merchantHandler.RegisterRoutes(mux)
	orderHandler.RegisterRoutesWithAuth(mux, protected)
	paymentHandler.RegisterRoutesWithAuth(mux, protected)
	settlementHandler.RegisterRoutesWithAuth(mux, protected)
	payoutHandler.RegisterRoutesWithAuth(mux, protected)
	sagaHandler.RegisterRoutesWithAuth(mux, protected)
	mux.Handle("GET /v1/merchants/me", authMw.RequireScope(merchant.APIKeyScopeRead, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := httpx.PrincipalFromContext(r.Context())
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"merchant_id": p.MerchantID})
	})))

	return sagaRuntimeEnv{
		mux:           mux,
		merchantSvc:   merchantSvc,
		orderSvc:      orderSvc,
		paymentSvc:    paymentSvc,
		settlementSvc: settlementSvc,
		payoutSvc:     payoutSvc,
		payoutRepo:    payoutRepo,
		sagaSvc:       sagaSvc,
	}
}

func seedPayoutSettlement(t *testing.T, ctx context.Context, merchantSvc *merchant.Service, orderSvc *order.Service, paymentSvc *payment.Service, settlementSvc *settlement.Service) (string, string, settlement.Settlement) {
	t.Helper()
	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Saga Runtime Merchant", Email: "runtime@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	orderOut, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: createdMerchant.ID,
		Amount:     13500,
		Currency:   "INR",
		Receipt:    "phase5-runtime",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	authOut, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID: createdMerchant.ID,
		OrderID:    orderOut.ID,
		Amount:     orderOut.Amount,
		Currency:   orderOut.Currency,
		Method:     "card",
	})
	if err != nil {
		t.Fatalf("authorize payment: %v", err)
	}
	if _, err := paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, authOut.PaymentID, orderOut.Amount); err != nil {
		t.Fatalf("capture payment: %v", err)
	}
	sttl, err := settlementSvc.RunBatch(ctx, createdMerchant.ID, time.Unix(0, 0), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("run settlement batch: %v", err)
	}
	return createdMerchant.ID, basicAuth(key.KeyID, key.KeySecret), sttl
}

func basicAuthFromMerchant(t *testing.T, merchantSvc *merchant.Service, merchantID string) string {
	t.Helper()
	key, err := merchantSvc.CreateAPIKey(context.Background(), merchantID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create auth key: %v", err)
	}
	return basicAuth(key.KeyID, key.KeySecret)
}

func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}
