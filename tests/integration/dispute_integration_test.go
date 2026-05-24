//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
	"github.com/sanskarpan/PayGate/internal/settlement"
)

func TestIntegrationDisputeLifecycleAndValidation(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	ctx := context.Background()

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Dispute Merchant", Email: "dispute@test.com", BusinessType: "company",
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
	cardTokenID := createCardTokenViaMux(t, mux, authHeader, false)

	o, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: createdMerchant.ID,
		Amount:     10000,
		Currency:   "INR",
		Receipt:    "disp-1",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	auth, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID:           createdMerchant.ID,
		OrderID:              o.ID,
		Amount:               o.Amount,
		Currency:             o.Currency,
		Method:               "card",
		PaymentMethodTokenID: cardTokenID,
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

	createBody, _ := json.Marshal(map[string]any{
		"payment_id":    auth.PaymentID,
		"settlement_id": sttl.ID,
		"reason":        "fraudulent",
		"amount":        o.Amount,
	})
	createReq := httptest.NewRequest(http.MethodPost, "/v1/disputes", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", authHeader)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create dispute: expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode dispute response: %v", err)
	}
	disputeID := created["id"].(string)
	if got := created["settlement_id"]; got != sttl.ID {
		t.Fatalf("expected settlement_id %s, got %v", sttl.ID, got)
	}
	if got := created["currency"]; got != "INR" {
		t.Fatalf("expected currency INR, got %v", got)
	}

	invalidReasonBody, _ := json.Marshal(map[string]any{
		"payment_id": auth.PaymentID,
		"reason":     "not_a_real_reason",
		"amount":     100,
	})
	invalidReasonReq := httptest.NewRequest(http.MethodPost, "/v1/disputes", bytes.NewReader(invalidReasonBody))
	invalidReasonReq.Header.Set("Authorization", authHeader)
	invalidReasonReq.Header.Set("Content-Type", "application/json")
	invalidReasonRec := httptest.NewRecorder()
	mux.ServeHTTP(invalidReasonRec, invalidReasonReq)
	if invalidReasonRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid reason: expected 400, got %d body=%s", invalidReasonRec.Code, invalidReasonRec.Body.String())
	}

	wrongCurrencyBody, _ := json.Marshal(map[string]any{
		"payment_id": auth.PaymentID,
		"reason":     "fraudulent",
		"amount":     100,
		"currency":   "USD",
	})
	wrongCurrencyReq := httptest.NewRequest(http.MethodPost, "/v1/disputes", bytes.NewReader(wrongCurrencyBody))
	wrongCurrencyReq.Header.Set("Authorization", authHeader)
	wrongCurrencyReq.Header.Set("Content-Type", "application/json")
	wrongCurrencyRec := httptest.NewRecorder()
	mux.ServeHTTP(wrongCurrencyRec, wrongCurrencyReq)
	if wrongCurrencyRec.Code != http.StatusBadRequest {
		t.Fatalf("wrong currency: expected 400, got %d body=%s", wrongCurrencyRec.Code, wrongCurrencyRec.Body.String())
	}

	evidenceBody, _ := json.Marshal(map[string]any{
		"proof_url": "https://example.com/evidence",
	})
	evidenceReq := httptest.NewRequest(http.MethodPost, "/v1/disputes/"+disputeID+"/submit-evidence", bytes.NewReader(evidenceBody))
	evidenceReq.Header.Set("Authorization", authHeader)
	evidenceReq.Header.Set("Content-Type", "application/json")
	evidenceRec := httptest.NewRecorder()
	mux.ServeHTTP(evidenceRec, evidenceReq)
	if evidenceRec.Code != http.StatusOK {
		t.Fatalf("submit evidence: expected 200, got %d body=%s", evidenceRec.Code, evidenceRec.Body.String())
	}

	reviewReq := httptest.NewRequest(http.MethodPost, "/v1/disputes/"+disputeID+"/review", nil)
	reviewReq.Header.Set("Authorization", authHeader)
	reviewRec := httptest.NewRecorder()
	mux.ServeHTTP(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusOK {
		t.Fatalf("review dispute: expected 200, got %d body=%s", reviewRec.Code, reviewRec.Body.String())
	}

	lateEvidenceReq := httptest.NewRequest(http.MethodPost, "/v1/disputes/"+disputeID+"/submit-evidence", bytes.NewReader(evidenceBody))
	lateEvidenceReq.Header.Set("Authorization", authHeader)
	lateEvidenceReq.Header.Set("Content-Type", "application/json")
	lateEvidenceRec := httptest.NewRecorder()
	mux.ServeHTTP(lateEvidenceRec, lateEvidenceReq)
	if lateEvidenceRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("late evidence: expected 422, got %d body=%s", lateEvidenceRec.Code, lateEvidenceRec.Body.String())
	}

	resolveBody, _ := json.Marshal(map[string]any{
		"outcome": "won",
		"notes":   "issuer reversed chargeback",
	})
	resolveReq := httptest.NewRequest(http.MethodPost, "/v1/disputes/"+disputeID+"/resolve", bytes.NewReader(resolveBody))
	resolveReq.Header.Set("Authorization", authHeader)
	resolveReq.Header.Set("Content-Type", "application/json")
	resolveRec := httptest.NewRecorder()
	mux.ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("resolve dispute: expected 200, got %d body=%s", resolveRec.Code, resolveRec.Body.String())
	}

	acceptReq := httptest.NewRequest(http.MethodPost, "/v1/disputes/"+disputeID+"/accept", nil)
	acceptReq.Header.Set("Authorization", authHeader)
	acceptRec := httptest.NewRecorder()
	mux.ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("accept terminal dispute: expected 422, got %d body=%s", acceptRec.Code, acceptRec.Body.String())
	}

	var createdEvents, wonEvents int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM public.outbox WHERE aggregate_id = $1 AND event_type = 'dispute.created'`, disputeID).Scan(&createdEvents); err != nil {
		t.Fatalf("count dispute.created events: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM public.outbox WHERE aggregate_id = $1 AND event_type = 'dispute.won'`, disputeID).Scan(&wonEvents); err != nil {
		t.Fatalf("count dispute.won events: %v", err)
	}
	if createdEvents != 1 || wonEvents != 1 {
		t.Fatalf("expected 1 dispute.created and 1 dispute.won event, got created=%d won=%d", createdEvents, wonEvents)
	}
}
