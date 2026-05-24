//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
	"github.com/sanskarpan/PayGate/internal/settlement"
)

func TestIntegrationReportingExportTaxProfileAndStatements(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Reporting Merchant", Email: "reporting@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	adminKey, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeAdmin})
	if err != nil {
		t.Fatalf("create admin key: %v", err)
	}
	writeKey, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeWrite})
	if err != nil {
		t.Fatalf("create write key: %v", err)
	}
	writeAuthHeader := basicAuth(writeKey.KeyID, writeKey.KeySecret)

	createdOrder, err := orderSvc.Create(ctx, order.CreateInput{MerchantID: createdMerchant.ID, Amount: 10000, Currency: "INR", Receipt: "report-1"})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	cardTokenID := createCardTokenViaMux(t, mux, writeAuthHeader, false)
	auth, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID:           createdMerchant.ID,
		OrderID:              createdOrder.ID,
		Amount:               createdOrder.Amount,
		Currency:             createdOrder.Currency,
		Method:               "card",
		PaymentMethodTokenID: cardTokenID,
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if _, err := paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, auth.PaymentID, createdOrder.Amount); err != nil {
		t.Fatalf("capture: %v", err)
	}

	start := time.Now().Add(-time.Hour).Unix()
	end := time.Now().Add(time.Hour).Unix()

	taxProfileBody := []byte(`{"legal_name":"Reporting Merchant Pvt Ltd","gstin":"29ABCDE1234F2Z5","business_state_code":"29","place_of_supply":"KA","default_tax_rate_bps":1800}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/reports/tax-profile", bytes.NewReader(taxProfileBody))
	req.Header.Set("Authorization", basicAuth(adminKey.KeyID, adminKey.KeySecret))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected tax profile 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	statementBody, _ := json.Marshal(map[string]any{"period_start": start, "period_end": end})
	req = httptest.NewRequest(http.MethodPost, "/v1/reports/statements/payments", bytes.NewReader(statementBody))
	req.Header.Set("Authorization", writeAuthHeader)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected statement 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var stmt map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &stmt); err != nil {
		t.Fatalf("decode statement: %v", err)
	}
	totals := stmt["totals"].(map[string]any)
	if totals["gross_amount"].(float64) != 10000 {
		t.Fatalf("expected gross 10000, got %#v", totals["gross_amount"])
	}
	if totals["tax_amount"].(float64) <= 0 {
		t.Fatalf("expected positive tax amount, got %#v", totals["tax_amount"])
	}

	exportBody, _ := json.Marshal(map[string]any{"report_type": "payments", "format": "csv", "period_start": start, "period_end": end})
	req = httptest.NewRequest(http.MethodPost, "/v1/reports/exports", bytes.NewReader(exportBody))
	req.Header.Set("Authorization", writeAuthHeader)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected export 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var export map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &export); err != nil {
		t.Fatalf("decode export response: %v", err)
	}
	downloadURL := export["download_url"].(string)
	token := downloadURL[strings.LastIndex(downloadURL, "token=")+6:]
	exportID := export["id"].(string)

	req = httptest.NewRequest(http.MethodGet, "/v1/reports/exports/"+exportID+"/download?token="+token, nil)
	req.Header.Set("Authorization", basicAuth(writeKey.KeyID, writeKey.KeySecret))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected export download 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), auth.PaymentID) {
		t.Fatalf("expected export csv to contain payment id %s", auth.PaymentID)
	}
}

func TestIntegrationReconSourceImportAssignmentAndResolution(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Recon Ops Merchant", Email: "recon-ops@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	adminKey, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeAdmin})
	if err != nil {
		t.Fatalf("create admin key: %v", err)
	}
	adminAuthHeader := basicAuth(adminKey.KeyID, adminKey.KeySecret)

	createdOrder, err := orderSvc.Create(ctx, order.CreateInput{MerchantID: createdMerchant.ID, Amount: 12000, Currency: "INR", Receipt: "recon-src-1"})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	cardTokenID := createCardTokenViaMux(t, mux, adminAuthHeader, false)
	authz, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID:           createdMerchant.ID,
		OrderID:              createdOrder.ID,
		Amount:               createdOrder.Amount,
		Currency:             createdOrder.Currency,
		Method:               "card",
		PaymentMethodTokenID: cardTokenID,
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if _, err := paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, authz.PaymentID, createdOrder.Amount); err != nil {
		t.Fatalf("capture: %v", err)
	}
	sttlSvc := settlement.NewService(settlement.NewPostgresRepository(db, ledger.NewService(ledger.NewRepository(db))))
	sttl, err := sttlSvc.RunBatch(ctx, createdMerchant.ID, time.Unix(0, 0), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("run settlement: %v", err)
	}

	importBody, _ := json.Marshal(map[string]any{
		"source_type":  "bank_settlement_file",
		"period_start": time.Now().Add(-time.Hour).Unix(),
		"period_end":   time.Now().Add(time.Hour).Unix(),
		"entries": []map[string]any{
			{
				"entity_type":  "settlement",
				"external_id":  "bank-row-1",
				"reference_id": sttl.ID,
				"amount":       sttl.NetAmount + 99,
				"currency":     sttl.Currency,
				"status":       string(sttl.Status),
				"occurred_at":  time.Now().Unix(),
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/recon/sources/import", bytes.NewReader(importBody))
	req.Header.Set("Authorization", adminAuthHeader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected import 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/recon/mismatches?unresolved=true", nil)
	req.Header.Set("Authorization", adminAuthHeader)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected mismatches 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var mismatches map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &mismatches); err != nil {
		t.Fatalf("decode mismatches: %v", err)
	}
	items := mismatches["items"].([]any)
	if len(items) == 0 {
		t.Fatal("expected recon mismatch after source import")
	}
	first := items[0].(map[string]any)
	mismatchID := first["id"].(string)

	assignBody := []byte(`{"assigned_to":"finance.ops","note":"requires bank statement review"}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/recon/mismatches/"+mismatchID+"/assign", bytes.NewReader(assignBody))
	req.Header.Set("Authorization", adminAuthHeader)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected assign 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/recon/mismatches/"+mismatchID+"/notes", bytes.NewReader([]byte(`{"author":"finance.ops","note":"confirmed upstream bank file issue"}`)))
	req.Header.Set("Authorization", adminAuthHeader)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected add note 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	resolveBody := []byte(`{"actor":"finance.ops","resolution_code":"bank_file_corrected","resolution_notes":"corrected by upstream partner"}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/recon/mismatches/"+mismatchID+"/resolve", bytes.NewReader(resolveBody))
	req.Header.Set("Authorization", adminAuthHeader)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected resolve 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/recon/mismatches/"+mismatchID+"/notes", nil)
	req.Header.Set("Authorization", basicAuth(adminKey.KeyID, adminKey.KeySecret))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected notes list 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIntegrationPaymentMethodStatesForCardAndUPI(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Method State Merchant", Email: "method-state@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeWrite})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHeader := basicAuth(key.KeyID, key.KeySecret)

	cardOrder, err := orderSvc.Create(ctx, order.CreateInput{MerchantID: createdMerchant.ID, Amount: 5000, Currency: "INR", Receipt: "card-ms"})
	if err != nil {
		t.Fatalf("create card order: %v", err)
	}
	cardTokenID := createCardTokenViaMux(t, mux, authHeader, false)
	cardAuth, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID:           createdMerchant.ID,
		OrderID:              cardOrder.ID,
		Amount:               cardOrder.Amount,
		Currency:             cardOrder.Currency,
		Method:               "card",
		PaymentMethodTokenID: cardTokenID,
	})
	if err != nil {
		t.Fatalf("authorize card: %v", err)
	}
	cardCaptured, err := paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, cardAuth.PaymentID, cardOrder.Amount)
	if err != nil {
		t.Fatalf("capture card: %v", err)
	}
	if cardCaptured.MethodState != payment.MethodStateCardCaptured {
		t.Fatalf("expected card method state %s, got %s", payment.MethodStateCardCaptured, cardCaptured.MethodState)
	}

	upiOrder, err := orderSvc.Create(ctx, order.CreateInput{MerchantID: createdMerchant.ID, Amount: 7000, Currency: "INR", Receipt: "upi-ms"})
	if err != nil {
		t.Fatalf("create upi order: %v", err)
	}
	intent, err := paymentSvc.CreateUPIIntent(ctx, payment.CreateUPIIntentInput{
		MerchantID: createdMerchant.ID,
		OrderID:    upiOrder.ID,
		Amount:     upiOrder.Amount,
		Currency:   upiOrder.Currency,
		VPA:        "customer@upi",
	})
	if err != nil {
		t.Fatalf("create upi intent: %v", err)
	}
	if intent.MethodState != payment.MethodStateUPIPendingCustomerAction {
		t.Fatalf("expected initial upi method state %s, got %s", payment.MethodStateUPIPendingCustomerAction, intent.MethodState)
	}
	if _, _, err := paymentSvc.ApplyUPICallback(ctx, createdMerchant.ID, intent.PaymentID, intent.CallbackToken, "evt_upi_1", "succeeded", "upi-ref-1", "", ""); err != nil {
		t.Fatalf("apply upi callback: %v", err)
	}
	updated, err := paymentSvc.GetUPIIntent(ctx, createdMerchant.ID, intent.PaymentID)
	if err != nil {
		t.Fatalf("get upi intent: %v", err)
	}
	if updated.MethodState != payment.MethodStateUPICollected {
		t.Fatalf("expected final upi method state %s, got %s", payment.MethodStateUPICollected, updated.MethodState)
	}
}
