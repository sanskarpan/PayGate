//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"github.com/sanskarpan/PayGate/internal/refund"
	"github.com/sanskarpan/PayGate/internal/settlement"
)

func TestIntegrationPayoutCompletesAndWritesLedger(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	ctx := context.Background()
	merchantEmail := uniqueTestEmail(t, "payout")

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Payout Merchant", Email: merchantEmail, BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeWrite,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHeader := basicAuth(key.KeyID, key.KeySecret)

	o, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: createdMerchant.ID,
		Amount:     10000,
		Currency:   "INR",
		Receipt:    "payout-1",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	auth, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID: createdMerchant.ID,
		OrderID:    o.ID,
		Amount:     o.Amount,
		Currency:   o.Currency,
		Method:     "card",
	})
	if err != nil {
		t.Fatalf("authorize payment: %v", err)
	}
	if _, err := paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, auth.PaymentID, o.Amount); err != nil {
		t.Fatalf("capture payment: %v", err)
	}

	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	settlementSvc := settlement.NewService(settlement.NewPostgresRepository(db, ledgerSvc))
	sttl, err := settlementSvc.RunBatch(ctx, createdMerchant.ID, time.Unix(0, 0), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("run settlement batch: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/settlements/"+sttl.ID+"/payout", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("initiate payout: expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payoutID string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := db.QueryRow(ctx, `
SELECT id
FROM paygate_payouts.payouts
WHERE merchant_id = $1 AND settlement_id = $2 AND status = 'completed'
`, createdMerchant.ID, sttl.ID).Scan(&payoutID); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if payoutID == "" {
		t.Fatal("expected payout to complete within 5s")
	}

	var bankReference string
	if err := db.QueryRow(ctx, `
SELECT bank_reference
FROM paygate_payouts.payouts
WHERE id = $1
`, payoutID).Scan(&bankReference); err != nil {
		t.Fatalf("query payout: %v", err)
	}
	if bankReference == "" {
		t.Fatal("expected completed payout to have a bank reference")
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
		t.Fatalf("expected 2 payout ledger entries, got %d", ledgerEntries)
	}
}

func TestIntegrationPayoutRailCallbackAuthenticityReplayAndTimeline(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, orderSvc, paymentSvc := buildPayoutRailMux(db, func(context.Context, string, string, string) (map[string]any, error) {
		return map[string]any{"status": "processing"}, nil
	})
	merchantID, authHeader, sttl := createSettledMerchantFlow(t, ctx, db, merchantSvc, orderSvc, paymentSvc)

	req := httptest.NewRequest(http.MethodPost, "/v1/settlements/"+sttl.ID+"/payout", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("initiate payout: expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var initiated map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &initiated); err != nil {
		t.Fatalf("decode payout create response: %v", err)
	}
	payoutID, _ := initiated["id"].(string)
	if payoutID == "" {
		t.Fatal("expected payout id in initiate response")
	}

	unsignedPayload := map[string]any{
		"event_id":       "rail_evt_auth_fail",
		"payout_id":      payoutID,
		"merchant_id":    merchantID,
		"status":         "completed",
		"rail_reference": "BNK_SIG_FAIL",
		"occurred_at":    time.Now().UTC().Format(time.RFC3339),
	}
	doRailCallback(t, mux, unsignedPayload, false)

	completedPayload := map[string]any{
		"event_id":       "rail_evt_complete_once",
		"payout_id":      payoutID,
		"merchant_id":    merchantID,
		"status":         "completed",
		"rail_reference": "BNK_RAIL_ONCE",
		"occurred_at":    time.Now().UTC().Format(time.RFC3339),
	}
	callbackRec := doRailCallback(t, mux, completedPayload, true)
	if callbackRec.Code != http.StatusAccepted {
		t.Fatalf("expected callback 202, got %d body=%s", callbackRec.Code, callbackRec.Body.String())
	}

	callbackRec = doRailCallback(t, mux, completedPayload, true)
	if callbackRec.Code != http.StatusAccepted {
		t.Fatalf("expected duplicate callback 202, got %d body=%s", callbackRec.Code, callbackRec.Body.String())
	}
	var callbackBody map[string]any
	if err := json.Unmarshal(callbackRec.Body.Bytes(), &callbackBody); err != nil {
		t.Fatalf("decode duplicate callback response: %v", err)
	}
	if processed, _ := callbackBody["processed"].(bool); processed {
		t.Fatalf("expected duplicate callback to report processed=false, got %#v", callbackBody)
	}

	var ledgerEntries int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM paygate_ledger.ledger_entries WHERE source_id = $1`, payoutID).Scan(&ledgerEntries); err != nil {
		t.Fatalf("count payout ledger entries: %v", err)
	}
	if ledgerEntries != 2 {
		t.Fatalf("expected exactly 2 payout ledger entries after duplicate callback replay, got %d", ledgerEntries)
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/v1/payouts/"+payoutID+"/events", nil)
	eventsReq.Header.Set("Authorization", authHeader)
	eventsRec := httptest.NewRecorder()
	mux.ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected payout events 200, got %d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
	if !bytes.Contains(eventsRec.Body.Bytes(), []byte(`"event_type":"payout.rail_completed"`)) {
		t.Fatalf("expected payout timeline to include rail completion event, got body=%s", eventsRec.Body.String())
	}
}

func TestIntegrationPayoutReturnReversesLedgerAndIgnoresOutOfOrderCompletion(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, orderSvc, paymentSvc := buildPayoutRailMux(db, func(context.Context, string, string, string) (map[string]any, error) {
		return map[string]any{"status": "processing"}, nil
	})
	merchantID, authHeader, sttl := createSettledMerchantFlow(t, ctx, db, merchantSvc, orderSvc, paymentSvc)

	req := httptest.NewRequest(http.MethodPost, "/v1/settlements/"+sttl.ID+"/payout", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("initiate payout: expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var initiated map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &initiated); err != nil {
		t.Fatalf("decode payout response: %v", err)
	}
	payoutID, _ := initiated["id"].(string)
	if payoutID == "" {
		t.Fatal("expected payout id")
	}

	completePayload := map[string]any{
		"event_id":       "rail_evt_complete_return_flow",
		"payout_id":      payoutID,
		"merchant_id":    merchantID,
		"status":         "completed",
		"rail_reference": "BNK_RETURN_FLOW",
		"occurred_at":    time.Now().UTC().Format(time.RFC3339),
	}
	completeRec := doRailCallback(t, mux, completePayload, true)
	if completeRec.Code != http.StatusAccepted {
		t.Fatalf("expected complete callback 202, got %d body=%s", completeRec.Code, completeRec.Body.String())
	}

	returnPayload := map[string]any{
		"event_id":       "rail_evt_return_once",
		"payout_id":      payoutID,
		"merchant_id":    merchantID,
		"status":         "returned",
		"rail_reference": "BNK_RETURN_FLOW",
		"reason":         "bank_account_closed",
		"occurred_at":    time.Now().UTC().Format(time.RFC3339),
	}
	returnRec := doRailCallback(t, mux, returnPayload, true)
	if returnRec.Code != http.StatusAccepted {
		t.Fatalf("expected returned callback 202, got %d body=%s", returnRec.Code, returnRec.Body.String())
	}

	returnRec = doRailCallback(t, mux, returnPayload, true)
	if returnRec.Code != http.StatusAccepted {
		t.Fatalf("expected duplicate returned callback 202, got %d body=%s", returnRec.Code, returnRec.Body.String())
	}

	lateCompletePayload := map[string]any{
		"event_id":       "rail_evt_complete_late",
		"payout_id":      payoutID,
		"merchant_id":    merchantID,
		"status":         "completed",
		"rail_reference": "BNK_RETURN_FLOW_LATE",
		"occurred_at":    time.Now().UTC().Format(time.RFC3339),
	}
	lateRec := doRailCallback(t, mux, lateCompletePayload, true)
	if lateRec.Code != http.StatusAccepted {
		t.Fatalf("expected late completed callback 202, got %d body=%s", lateRec.Code, lateRec.Body.String())
	}

	var payoutStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM paygate_payouts.payouts WHERE id = $1`, payoutID).Scan(&payoutStatus); err != nil {
		t.Fatalf("query payout status: %v", err)
	}
	if payoutStatus != string(payout.StateReturned) {
		t.Fatalf("expected payout status returned, got %s", payoutStatus)
	}

	var ledgerEntries int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM paygate_ledger.ledger_entries WHERE source_id = $1`, payoutID).Scan(&ledgerEntries); err != nil {
		t.Fatalf("count payout ledger entries: %v", err)
	}
	if ledgerEntries != 4 {
		t.Fatalf("expected exactly 4 ledger entries after return reversal, got %d", ledgerEntries)
	}

	var settlementClearingNet int64
	if err := db.QueryRow(ctx, `
SELECT COALESCE(SUM(CASE WHEN account_code = 'SETTLEMENT_CLEARING' THEN debit_amount - credit_amount ELSE 0 END), 0)
FROM paygate_ledger.ledger_entries
WHERE source_id = $1
`, payoutID).Scan(&settlementClearingNet); err != nil {
		t.Fatalf("query settlement clearing net: %v", err)
	}
	if settlementClearingNet != 0 {
		t.Fatalf("expected settlement clearing net 0 after payout return, got %d", settlementClearingNet)
	}

	var merchantBankNet int64
	if err := db.QueryRow(ctx, `
SELECT COALESCE(SUM(CASE WHEN account_code = 'MERCHANT_BANK_PAYOUT' THEN debit_amount - credit_amount ELSE 0 END), 0)
FROM paygate_ledger.ledger_entries
WHERE source_id = $1
`, payoutID).Scan(&merchantBankNet); err != nil {
		t.Fatalf("query merchant bank payout net: %v", err)
	}
	if merchantBankNet != 0 {
		t.Fatalf("expected merchant bank payout net 0 after payout return, got %d", merchantBankNet)
	}
}

func TestIntegrationPayoutSimulatorScenarioExercisesRetryOutOfOrderDuplicateAndReturn(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	_, authHeader, sttl := createSettledMerchantFlow(t, ctx, db, merchantSvc, orderSvc, paymentSvc)

	getScenarioReq := httptest.NewRequest(http.MethodGet, "/v1/settlements/"+sttl.ID+"/payout-simulator", nil)
	getScenarioReq.Header.Set("Authorization", authHeader)
	getScenarioRec := httptest.NewRecorder()
	mux.ServeHTTP(getScenarioRec, getScenarioReq)
	if getScenarioRec.Code != http.StatusOK {
		t.Fatalf("expected default payout simulator scenario 200, got %d body=%s", getScenarioRec.Code, getScenarioRec.Body.String())
	}

	var defaultScenario map[string]any
	if err := json.Unmarshal(getScenarioRec.Body.Bytes(), &defaultScenario); err != nil {
		t.Fatalf("decode default simulator scenario: %v", err)
	}
	defaultSteps, _ := defaultScenario["steps"].([]any)
	if len(defaultSteps) != 2 {
		t.Fatalf("expected 2 default simulator steps, got %#v", defaultScenario["steps"])
	}

	scenarioBody := []byte(`{
		"transient_failures_remaining": 1,
		"notes": "simulate transient rail failure, out-of-order callbacks, and returned funds",
		"steps": [
			{"status":"completed","rail_reference":"BNK_OUT_OF_ORDER","delay_ms":25},
			{"status":"processing","delay_ms":10,"duplicate_count":1},
			{"status":"returned","rail_reference":"BNK_OUT_OF_ORDER","reason":"beneficiary_account_closed","duplicate_count":1}
		]
	}`)
	putScenarioReq := httptest.NewRequest(http.MethodPut, "/v1/settlements/"+sttl.ID+"/payout-simulator", bytes.NewReader(scenarioBody))
	putScenarioReq.Header.Set("Authorization", authHeader)
	putScenarioReq.Header.Set("Content-Type", "application/json")
	putScenarioRec := httptest.NewRecorder()
	mux.ServeHTTP(putScenarioRec, putScenarioReq)
	if putScenarioRec.Code != http.StatusOK {
		t.Fatalf("expected simulator scenario update 200, got %d body=%s", putScenarioRec.Code, putScenarioRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/settlements/"+sttl.ID+"/payout", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("initiate payout: expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var initiated map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &initiated); err != nil {
		t.Fatalf("decode payout initiate response: %v", err)
	}
	payoutID, _ := initiated["id"].(string)
	sagaID, _ := initiated["saga_id"].(string)
	if payoutID == "" || sagaID == "" {
		t.Fatalf("expected payout id and saga id, got payout=%q saga=%q body=%s", payoutID, sagaID, rec.Body.String())
	}

	var payoutStatus string
	var returnReason string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		err := db.QueryRow(ctx, `
SELECT status, COALESCE(return_reason, '')
FROM paygate_payouts.payouts
WHERE id = $1
`, payoutID).Scan(&payoutStatus, &returnReason)
		if err == nil && payoutStatus == string(payout.StateReturned) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if payoutStatus != string(payout.StateReturned) {
		t.Fatalf("expected payout to reach returned state, got %q", payoutStatus)
	}
	if returnReason != "beneficiary_account_closed" {
		t.Fatalf("expected bank return reason to persist, got %q", returnReason)
	}

	dispatchReq := httptest.NewRequest(http.MethodGet, "/v1/sagas/"+sagaID+"/dispatches", nil)
	dispatchReq.Header.Set("Authorization", authHeader)
	dispatchRec := httptest.NewRecorder()
	mux.ServeHTTP(dispatchRec, dispatchReq)
	if dispatchRec.Code != http.StatusOK {
		t.Fatalf("expected saga dispatches 200, got %d body=%s", dispatchRec.Code, dispatchRec.Body.String())
	}
	var dispatchBody struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(dispatchRec.Body.Bytes(), &dispatchBody); err != nil {
		t.Fatalf("decode dispatches response: %v", err)
	}
	if len(dispatchBody.Items) < 2 {
		t.Fatalf("expected at least two dispatch attempts after transient failure, got %#v", dispatchBody.Items)
	}
	var sawAcked bool
	var sawNacked bool
	for _, item := range dispatchBody.Items {
		status, _ := item["status"].(string)
		switch status {
		case "acked":
			sawAcked = true
		case "nacked":
			sawNacked = true
		}
	}
	if !sawAcked || !sawNacked {
		t.Fatalf("expected dispatch history to include both acked and nacked attempts, got %#v", dispatchBody.Items)
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/v1/payouts/"+payoutID+"/events", nil)
	eventsReq.Header.Set("Authorization", authHeader)
	eventsRec := httptest.NewRecorder()
	mux.ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected payout events 200, got %d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
	for _, fragment := range []string{
		`"event_type":"payout.rail_completed"`,
		`"event_type":"payout.rail_processing"`,
		`"event_type":"payout.rail_returned"`,
	} {
		if !bytes.Contains(eventsRec.Body.Bytes(), []byte(fragment)) {
			t.Fatalf("expected payout timeline to include %s, got body=%s", fragment, eventsRec.Body.String())
		}
	}

	getScenarioReq = httptest.NewRequest(http.MethodGet, "/v1/settlements/"+sttl.ID+"/payout-simulator", nil)
	getScenarioReq.Header.Set("Authorization", authHeader)
	getScenarioRec = httptest.NewRecorder()
	mux.ServeHTTP(getScenarioRec, getScenarioReq)
	if getScenarioRec.Code != http.StatusOK {
		t.Fatalf("expected payout simulator scenario fetch 200, got %d body=%s", getScenarioRec.Code, getScenarioRec.Body.String())
	}
	var updatedScenario map[string]any
	if err := json.Unmarshal(getScenarioRec.Body.Bytes(), &updatedScenario); err != nil {
		t.Fatalf("decode updated simulator scenario: %v", err)
	}
	if remaining, _ := updatedScenario["transient_failures_remaining"].(float64); remaining != 0 {
		t.Fatalf("expected simulator transient failures to be consumed, got %#v", updatedScenario)
	}

	var callbackReceipts int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM paygate_payouts.payout_callback_receipts WHERE payout_id = $1`, payoutID).Scan(&callbackReceipts); err != nil {
		t.Fatalf("count payout callback receipts: %v", err)
	}
	if callbackReceipts != 3 {
		t.Fatalf("expected exactly 3 unique callback receipts after duplicate simulation, got %d", callbackReceipts)
	}

	var ledgerEntries int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM paygate_ledger.ledger_entries WHERE source_id = $1`, payoutID).Scan(&ledgerEntries); err != nil {
		t.Fatalf("count payout ledger entries: %v", err)
	}
	if ledgerEntries != 4 {
		t.Fatalf("expected 4 ledger entries after completion and return reversal, got %d", ledgerEntries)
	}

	var settlementClearingNet int64
	if err := db.QueryRow(ctx, `
SELECT COALESCE(SUM(CASE WHEN account_code = 'SETTLEMENT_CLEARING' THEN debit_amount - credit_amount ELSE 0 END), 0)
FROM paygate_ledger.ledger_entries
WHERE source_id = $1
`, payoutID).Scan(&settlementClearingNet); err != nil {
		t.Fatalf("query settlement clearing net: %v", err)
	}
	if settlementClearingNet != 0 {
		t.Fatalf("expected settlement clearing net 0 after simulated return, got %d", settlementClearingNet)
	}
}

func createSettledMerchantFlow(t *testing.T, ctx context.Context, db *pgxpool.Pool, merchantSvc *merchant.Service, orderSvc *order.Service, paymentSvc *payment.Service) (string, string, settlement.Settlement) {
	t.Helper()
	merchantEmail := uniqueTestEmail(t, "payout-rail")
	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Payout Rail Merchant", Email: merchantEmail, BusinessType: "company",
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
		Amount:     10000,
		Currency:   "INR",
		Receipt:    "payout-rail-order",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	auth, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID: createdMerchant.ID,
		OrderID:    o.ID,
		Amount:     o.Amount,
		Currency:   o.Currency,
		Method:     "card",
	})
	if err != nil {
		t.Fatalf("authorize payment: %v", err)
	}
	if _, err := paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, auth.PaymentID, o.Amount); err != nil {
		t.Fatalf("capture payment: %v", err)
	}

	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	settlementSvc := settlement.NewService(settlement.NewPostgresRepository(db, ledgerSvc))
	sttl, err := settlementSvc.RunBatch(ctx, createdMerchant.ID, time.Unix(0, 0), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("run settlement batch: %v", err)
	}
	return createdMerchant.ID, authHeader, sttl
}

func doRailCallback(t *testing.T, mux *http.ServeMux, payload map[string]any, signed bool) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal rail payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/payouts/rail/callbacks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	timestamp := time.Now().UTC().Format(time.RFC3339)
	req.Header.Set("X-PayGate-Rail-Timestamp", timestamp)
	if signed {
		req.Header.Set("X-PayGate-Rail-Signature", payout.SignRailPayload("paygate-test-payout-rail-secret", timestamp, body))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if !signed && rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unsigned rail callback to be rejected with 401, got %d body=%s", rec.Code, rec.Body.String())
	}
	return rec
}

func buildPayoutRailMux(db *pgxpool.Pool, transferFn func(context.Context, string, string, string) (map[string]any, error)) (*http.ServeMux, *merchant.Service, *order.Service, *payment.Service) {
	merchantRepo := merchant.NewPostgresRepository(db)
	merchantSvc := merchant.NewService(merchantRepo, merchant.WithSessionSecret("integration-dashboard-secret"))
	authMw := auth.NewMiddleware(merchantSvc)
	idemMw := idempotency.NewMiddleware(idempotency.NewStore(db, nil))

	orderSvc := order.NewService(order.NewPostgresRepository(db))
	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	paymentSvc := payment.NewService(payment.NewPostgresRepository(db, ledgerSvc, orderSvc), gateway.NewSimulator())
	refundSvc := refund.NewService(refund.NewPostgresRepository(db, ledgerSvc))
	settlementSvc := settlement.NewService(settlement.NewPostgresRepository(db, ledgerSvc))
	payoutSvc := payout.NewService(payout.NewPostgresRepository(db, ledgerSvc), nil)
	payoutSvc.SetLedgerService(ledgerSvc)
	payoutSvc.SetRailCallbackSecret("paygate-test-payout-rail-secret")
	payoutSvc.SetTransferExecutorForTest(transferFn)

	merchantHandler := merchant.NewHandler(merchantSvc)
	orderHandler := order.NewHandler(orderSvc)
	paymentHandler := payment.NewHandler(paymentSvc)
	refundHandler := refund.NewHandler(refundSvc)
	settlementHandler := settlement.NewHandler(settlementSvc)
	payoutHandler := payout.NewHandler(payoutSvc, settlementSvc, ledgerSvc)

	protected := func(scope merchant.APIKeyScope, next http.Handler) http.Handler {
		return authMw.RequireScope(scope, idemMw.Wrap(next))
	}

	mux := http.NewServeMux()
	merchantHandler.RegisterRoutes(mux)
	merchantHandler.RegisterProtectedRoutes(mux, protected)
	orderHandler.RegisterRoutesWithAuth(mux, protected)
	paymentHandler.RegisterRoutesWithAuth(mux, protected)
	refundHandler.RegisterRoutesWithAuth(mux, protected)
	settlementHandler.RegisterRoutesWithAuth(mux, protected)
	payoutHandler.RegisterPublicRoutes(mux)
	payoutHandler.RegisterRoutesWithAuth(mux, protected)
	mux.Handle("GET /v1/merchants/me", authMw.RequireScope(merchant.APIKeyScopeRead, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := httpx.PrincipalFromContext(r.Context())
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"merchant_id": p.MerchantID})
	})))
	return mux, merchantSvc, orderSvc, paymentSvc
}

func uniqueTestEmail(t *testing.T, prefix string) string {
	t.Helper()
	slug := strings.NewReplacer("/", "-", " ", "-", ":", "-").Replace(t.Name())
	return fmt.Sprintf("%s-%s-%d@test.com", prefix, slug, time.Now().UnixNano())
}
