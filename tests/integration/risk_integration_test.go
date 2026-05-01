//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/auth"
	"github.com/sanskarpan/PayGate/internal/gateway"
	"github.com/sanskarpan/PayGate/internal/idempotency"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
	"github.com/sanskarpan/PayGate/internal/risk"
)

func buildRiskMux(db *pgxpool.Pool) (*http.ServeMux, *merchant.Service, *risk.Service) {
	merchantRepo := merchant.NewPostgresRepository(db)
	merchantSvc := merchant.NewService(merchantRepo, merchant.WithSessionSecret("risk-test-secret"))
	authMw := auth.NewMiddleware(merchantSvc)
	idemMw := idempotency.NewMiddleware(idempotency.NewStore(db, nil))

	riskRepo := risk.NewPostgresRepository(db)
	riskSvc := risk.NewService(riskRepo, nil)
	riskHandler := risk.NewHandler(riskSvc)

	orderSvc := order.NewService(order.NewPostgresRepository(db))
	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	paymentSvc := payment.NewService(
		payment.NewPostgresRepository(db, ledgerSvc, orderSvc),
		gateway.NewSimulator(),
	)
	paymentHandler := payment.NewHandler(paymentSvc, payment.WithRiskEvaluator(&testRiskAdapter{svc: riskSvc}))
	merchantHandler := merchant.NewHandler(merchantSvc)
	orderHandler := order.NewHandler(orderSvc)

	protected := func(scope merchant.APIKeyScope, next http.Handler) http.Handler {
		return authMw.RequireScope(scope, idemMw.Wrap(next))
	}

	mux := http.NewServeMux()
	merchantHandler.RegisterRoutes(mux)
	merchantHandler.RegisterProtectedRoutes(mux, protected)
	orderHandler.RegisterRoutesWithAuth(mux, protected)
	paymentHandler.RegisterRoutesWithAuth(mux, protected)
	riskHandler.RegisterRoutesWithAuth(mux, func(scope string, next http.Handler) http.Handler {
		return authMw.RequireScope(merchant.APIKeyScope(scope), next)
	})
	return mux, merchantSvc, riskSvc
}

// testRiskAdapter wraps risk.Service for use as payment.RiskEvaluator.
type testRiskAdapter struct{ svc *risk.Service }

func (a *testRiskAdapter) EvaluateAuthorize(ctx context.Context, merchantID, paymentID string, amount int64, ip string) (string, error) {
	ev, err := a.svc.EvaluatePayment(ctx, risk.EvalInput{
		MerchantID: merchantID,
		PaymentID:  paymentID,
		Amount:     amount,
		Currency:   "INR",
		IPAddress:  ip,
	})
	if err != nil {
		return string(risk.RiskActionAllow), err
	}
	return string(ev.Action), nil
}

func TestIntegrationRiskEventRecordedOnPaymentAuthorize(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _ := buildRiskMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Risk Merchant", Email: "risk@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, m.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeWrite,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHdr := basicAuth(key.KeyID, key.KeySecret)

	// Create an order.
	orderBody, _ := json.Marshal(map[string]any{"amount": 5000, "currency": "INR", "receipt": "risk-order-1"})
	orderReq := httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewReader(orderBody))
	orderReq.Header.Set("Authorization", authHdr)
	orderReq.Header.Set("Content-Type", "application/json")
	orderRec := httptest.NewRecorder()
	mux.ServeHTTP(orderRec, orderReq)
	if orderRec.Code != http.StatusCreated {
		t.Fatalf("create order: expected 201, got %d body=%s", orderRec.Code, orderRec.Body.String())
	}
	var orderResp map[string]any
	if err := json.Unmarshal(orderRec.Body.Bytes(), &orderResp); err != nil {
		t.Fatalf("decode order response: %v", err)
	}

	// Authorize a payment — risk event should be recorded.
	payBody, _ := json.Marshal(map[string]any{
		"order_id": orderResp["id"],
		"amount":   5000,
		"currency": "INR",
		"method":   "card",
	})
	payReq := httptest.NewRequest(http.MethodPost, "/v1/payments/authorize", bytes.NewReader(payBody))
	payReq.Header.Set("Authorization", authHdr)
	payReq.Header.Set("Content-Type", "application/json")
	payRec := httptest.NewRecorder()
	mux.ServeHTTP(payRec, payReq)
	if payRec.Code != http.StatusCreated {
		t.Fatalf("authorize payment: expected 201, got %d body=%s", payRec.Code, payRec.Body.String())
	}

	// Verify risk event exists for the payment.
	var payResp map[string]any
	if err := json.Unmarshal(payRec.Body.Bytes(), &payResp); err != nil {
		t.Fatalf("decode payment response: %v", err)
	}
	paymentID := payResp["id"].(string)

	var count int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM paygate_risk.risk_events WHERE payment_id = $1`, paymentID).Scan(&count); err != nil {
		t.Fatalf("query risk event count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 risk event, got %d", count)
	}
}

func TestIntegrationRiskEventListAndResolve(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, riskSvc := buildRiskMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Risk Resolve Merchant", Email: "riskresolve@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, m.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHdr := basicAuth(key.KeyID, key.KeySecret)

	// Directly create a risk event via the service.
	ev, err := riskSvc.EvaluatePayment(ctx, risk.EvalInput{
		MerchantID:     m.ID,
		PaymentID:      "pay_risk_test",
		Amount:         100000,
		Currency:       "INR",
		MerchantAvgTxn: 1000,
	})
	if err != nil {
		t.Fatalf("evaluate payment: %v", err)
	}

	// List risk events.
	listReq := httptest.NewRequest(http.MethodGet, "/v1/risk/events", nil)
	listReq.Header.Set("Authorization", authHdr)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list risk events: expected 200, got %d", listRec.Code)
	}
	var listResp map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if int(listResp["count"].(float64)) < 1 {
		t.Fatal("expected at least one risk event in list")
	}

	// Resolve the risk event.
	resolveReq := httptest.NewRequest(http.MethodPost, "/v1/risk/events/"+ev.ID+"/resolve",
		bytes.NewReader([]byte(`{"resolved_by":"admin_user"}`)))
	resolveReq.Header.Set("Authorization", authHdr)
	resolveReq.Header.Set("Content-Type", "application/json")
	resolveRec := httptest.NewRecorder()
	mux.ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("resolve: expected 200, got %d body=%s", resolveRec.Code, resolveRec.Body.String())
	}

	// Verify resolved in DB.
	var resolved bool
	if err := db.QueryRow(ctx, `SELECT resolved FROM paygate_risk.risk_events WHERE id = $1`, ev.ID).Scan(&resolved); err != nil {
		t.Fatalf("query resolved: %v", err)
	}
	if !resolved {
		t.Fatal("expected risk event to be resolved")
	}
}

func TestIntegrationAmountSpikeDetected(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	riskRepo := risk.NewPostgresRepository(db)
	riskSvc := risk.NewService(riskRepo, nil)

	merchantID := "merch_risk_spike_test"
	_, _ = db.Exec(ctx, `
		INSERT INTO paygate_merchants.merchants(id,name,email,business_type,status,settings)
		VALUES($1,'SpikeMerch','spike@test.com','company','active','{}') ON CONFLICT (id) DO NOTHING
	`, merchantID)

	// Evaluate a 10x spike payment without DB average (MerchantAvgTxn=0 triggers DB lookup,
	// which will return 0 so spike won't fire — use explicit avg instead).
	ev, err := riskSvc.EvaluatePayment(ctx, risk.EvalInput{
		MerchantID:     merchantID,
		PaymentID:      "pay_spike_1",
		Amount:         risk.ThresholdAmountSpikeMinAvg * risk.ThresholdAmountSpikeFactor * 10,
		Currency:       "INR",
		MerchantAvgTxn: risk.ThresholdAmountSpikeMinAvg,
	})
	if err != nil {
		t.Fatalf("evaluate spike payment: %v", err)
	}
	if ev.Action != risk.RiskActionHold && ev.Action != risk.RiskActionBlock {
		t.Fatalf("expected hold or block for spike payment, got %s (score=%d)", ev.Action, ev.Score)
	}
	found := false
	for _, r := range ev.TriggeredRules {
		if r == "amount_spike_3x" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected amount_spike_3x rule, got rules=%v", ev.TriggeredRules)
	}
}
