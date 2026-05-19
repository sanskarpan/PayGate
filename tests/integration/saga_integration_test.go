//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

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

func TestIntegrationSagaReplayCompletesFailedPayout(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	merchantRepo := merchant.NewPostgresRepository(db)
	merchantSvc := merchant.NewService(merchantRepo, merchant.WithSessionSecret("integration-dashboard-secret"))
	authMw := auth.NewMiddleware(merchantSvc)
	idemMw := idempotency.NewMiddleware(idempotency.NewStore(db, nil))

	orderSvc := order.NewService(order.NewPostgresRepository(db))
	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	paymentSvc := payment.NewService(payment.NewPostgresRepository(db, ledgerSvc, orderSvc), gateway.NewSimulator())
	settlementSvc := settlement.NewService(settlement.NewPostgresRepository(db, ledgerSvc))

	sagaSvc := saga.NewService(saga.NewPostgresRepository(db), nil)
	go saga.NewWorker(sagaSvc, 10*time.Millisecond, nil).Start(context.Background())

	payoutRepo := payout.NewPostgresRepository(db, ledgerSvc)
	payoutSvc := payout.NewService(payoutRepo, nil)
	payoutSvc.EnableSagaOrchestration(sagaSvc)
	payoutSvc.RegisterSagaHandlers(sagaSvc)

	var failFirst atomic.Int32
	failFirst.Store(1)
	payoutSvc.SetTransferExecutorForTest(func(ctx context.Context, merchantID, payoutID, commandID string) (map[string]any, error) {
		if failFirst.CompareAndSwap(1, 0) {
			return nil, errors.New("simulated rail failure")
		}
		out, err := payoutRepo.Complete(ctx, merchantID, payoutID, "BNK_REPLAY_OK")
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"payout_id":      out.ID,
			"bank_reference": out.BankReference,
			"status":         out.Status,
		}, nil
	})

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

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Saga Merchant", Email: "saga@test.com", BusinessType: "company",
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
	authHeader := basicAuth(key.KeyID, key.KeySecret)

	o, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: createdMerchant.ID,
		Amount:     12000,
		Currency:   "INR",
		Receipt:    "saga-replay",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	authOut, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID: createdMerchant.ID,
		OrderID:    o.ID,
		Amount:     o.Amount,
		Currency:   o.Currency,
		Method:     "card",
	})
	if err != nil {
		t.Fatalf("authorize payment: %v", err)
	}
	if _, err := paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, authOut.PaymentID, o.Amount); err != nil {
		t.Fatalf("capture payment: %v", err)
	}

	sttl, err := settlementSvc.RunBatch(ctx, createdMerchant.ID, time.Unix(0, 0), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("run settlement batch: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/settlements/"+sttl.ID+"/payout", nil)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("X-Request-Id", "corr-saga-001")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("initiate payout: expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payoutID, sagaID string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		err := db.QueryRow(ctx, `
SELECT id, saga_id
FROM paygate_payouts.payouts
WHERE merchant_id = $1 AND settlement_id = $2 AND status = 'processing'
`, createdMerchant.ID, sttl.ID).Scan(&payoutID, &sagaID)
		if err == nil && payoutID != "" && sagaID != "" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if payoutID == "" || sagaID == "" {
		t.Fatal("expected processing payout with attached failed saga")
	}

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var sagaStatus string
		err := db.QueryRow(ctx, `
SELECT status
FROM paygate_sagas.saga_instances
WHERE id = $1
`, sagaID).Scan(&sagaStatus)
		if err == nil && sagaStatus == string(saga.StatusFailed) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/sagas/"+sagaID, nil)
	getReq.Header.Set("Authorization", authHeader)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get saga: expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var sagaResp map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &sagaResp); err != nil {
		t.Fatalf("decode saga response: %v", err)
	}
	if sagaResp["status"] != string(saga.StatusFailed) {
		t.Fatalf("expected failed saga before replay, got %v", sagaResp["status"])
	}

	replayReq := httptest.NewRequest(http.MethodPost, "/v1/sagas/"+sagaID+"/replay", nil)
	replayReq.Header.Set("Authorization", authHeader)
	replayRec := httptest.NewRecorder()
	mux.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusAccepted {
		t.Fatalf("replay saga: expected 202, got %d body=%s", replayRec.Code, replayRec.Body.String())
	}

	deadline = time.Now().Add(5 * time.Second)
	var replayCount int
	for time.Now().Before(deadline) {
		var payoutStatus string
		var sagaStatus string
		var bankReference string
		err := db.QueryRow(ctx, `
SELECT p.status, p.bank_reference, s.status, s.replay_count
FROM paygate_payouts.payouts p
JOIN paygate_sagas.saga_instances s ON s.id = p.saga_id
WHERE p.id = $1
`, payoutID).Scan(&payoutStatus, &bankReference, &sagaStatus, &replayCount)
		if err == nil && payoutStatus == "completed" && sagaStatus == "completed" && bankReference == "BNK_REPLAY_OK" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if replayCount != 1 {
		t.Fatalf("expected replay_count=1, got %d", replayCount)
	}

	var ledgerEntries int
	if err := db.QueryRow(ctx, `
SELECT COUNT(*)
FROM paygate_ledger.ledger_entries
WHERE source_id = $1
`, payoutID).Scan(&ledgerEntries); err != nil {
		t.Fatalf("count payout ledger entries: %v", err)
	}
	if ledgerEntries != 2 {
		t.Fatalf("expected 2 payout ledger entries after replay success, got %d", ledgerEntries)
	}
}
