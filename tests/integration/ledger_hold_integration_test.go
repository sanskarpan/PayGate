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

func TestIntegrationLedgerHoldCommitPostsExactlyOnce(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, _, _ := buildGatewayMux(db)

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Hold Commit Merchant", Email: "hold-commit@test.com", BusinessType: "company",
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
	authHeader := basicAuth(key.KeyID, key.KeySecret)

	createBody := []byte(`{"account_code":"SETTLEMENT_CLEARING","source_type":"payout_hold","source_id":"settlement_demo","reason":"ops review","currency":"INR","amount":4900}`)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/ledger/holds", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", authHeader)
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Idempotency-Key", "hold-create-1")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create hold: expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode hold create: %v", err)
	}
	holdID, _ := created["id"].(string)
	if holdID == "" {
		t.Fatal("expected hold id in create response")
	}

	commitBody := []byte(`{"target_account_code":"MERCHANT_BANK_PAYOUT","description":"release payout funds"}`)
	commitReq := httptest.NewRequest(http.MethodPost, "/v1/ledger/holds/"+holdID+"/commit", bytes.NewReader(commitBody))
	commitReq.Header.Set("Authorization", authHeader)
	commitReq.Header.Set("Content-Type", "application/json")
	commitRec := httptest.NewRecorder()
	mux.ServeHTTP(commitRec, commitReq)
	if commitRec.Code != http.StatusOK {
		t.Fatalf("commit hold: expected 200, got %d body=%s", commitRec.Code, commitRec.Body.String())
	}

	var status string
	if err := db.QueryRow(ctx, `
SELECT status
FROM paygate_ledger.ledger_holds
WHERE id = $1
`, holdID).Scan(&status); err != nil {
		t.Fatalf("query hold status: %v", err)
	}
	if status != "committed" {
		t.Fatalf("expected committed hold, got %s", status)
	}

	var entries int
	if err := db.QueryRow(ctx, `
SELECT COUNT(*)
FROM paygate_ledger.ledger_entries
WHERE source_type = 'ledger_hold_commit' AND source_id = $1
`, holdID).Scan(&entries); err != nil {
		t.Fatalf("count hold commit entries: %v", err)
	}
	if entries != 2 {
		t.Fatalf("expected 2 hold commit ledger entries, got %d", entries)
	}

	repeatReq := httptest.NewRequest(http.MethodPost, "/v1/ledger/holds/"+holdID+"/commit", bytes.NewReader(commitBody))
	repeatReq.Header.Set("Authorization", authHeader)
	repeatReq.Header.Set("Content-Type", "application/json")
	repeatRec := httptest.NewRecorder()
	mux.ServeHTTP(repeatRec, repeatReq)
	if repeatRec.Code != http.StatusOK {
		t.Fatalf("repeat commit: expected 200, got %d body=%s", repeatRec.Code, repeatRec.Body.String())
	}

	var entriesAfter int
	if err := db.QueryRow(ctx, `
SELECT COUNT(*)
FROM paygate_ledger.ledger_entries
WHERE source_type = 'ledger_hold_commit' AND source_id = $1
`, holdID).Scan(&entriesAfter); err != nil {
		t.Fatalf("recount hold commit entries: %v", err)
	}
	if entriesAfter != 2 {
		t.Fatalf("expected 2 hold commit ledger entries after duplicate commit, got %d", entriesAfter)
	}
}

func TestIntegrationPayoutBlockedByActiveLedgerHoldUntilRelease(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Hold Block Merchant", Email: "hold-block@test.com", BusinessType: "company",
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
	authHeader := basicAuth(key.KeyID, key.KeySecret)
	adminKey, err := merchantSvc.CreateAPIKey(ctx, m.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create admin api key: %v", err)
	}
	adminAuthHeader := basicAuth(adminKey.KeyID, adminKey.KeySecret)

	o, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: m.ID, Amount: 10000, Currency: "INR", Receipt: "ledger-hold-payout",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	cardTokenID := createCardTokenViaMux(t, mux, authHeader, false)
	authOut, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID:           m.ID,
		OrderID:              o.ID,
		Amount:               o.Amount,
		Currency:             o.Currency,
		Method:               "card",
		PaymentMethodTokenID: cardTokenID,
	})
	if err != nil {
		t.Fatalf("authorize payment: %v", err)
	}
	if _, err := paymentSvc.CaptureForMerchant(ctx, m.ID, authOut.PaymentID, o.Amount); err != nil {
		t.Fatalf("capture payment: %v", err)
	}

	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	settlementSvc := settlement.NewService(settlement.NewPostgresRepository(db, ledgerSvc))
	sttl, err := settlementSvc.RunBatch(ctx, m.ID, time.Unix(0, 0), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("run settlement batch: %v", err)
	}

	createBody, _ := json.Marshal(map[string]any{
		"account_code": "SETTLEMENT_CLEARING",
		"source_type":  "payout_hold",
		"source_id":    sttl.ID,
		"reason":       "manual finance review",
		"currency":     sttl.Currency,
		"amount":       sttl.NetAmount,
	})
	createReq := httptest.NewRequest(http.MethodPost, "/v1/ledger/holds", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", authHeader)
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Idempotency-Key", "hold-create-payout-block")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create hold: expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var holdResp map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &holdResp); err != nil {
		t.Fatalf("decode hold create response: %v", err)
	}
	holdID, _ := holdResp["id"].(string)
	if holdID == "" {
		t.Fatal("expected hold id")
	}

	beneficiaryID := createApprovedBeneficiary(t, mux, adminAuthHeader)
	payoutReq := httptest.NewRequest(http.MethodPost, "/v1/settlements/"+sttl.ID+"/payout", bytes.NewReader([]byte(`{"beneficiary_id":"`+beneficiaryID+`"}`)))
	payoutReq.Header.Set("Content-Type", "application/json")
	payoutReq.Header.Set("Authorization", authHeader)
	payoutRec := httptest.NewRecorder()
	mux.ServeHTTP(payoutRec, payoutReq)
	if payoutRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 when ledger hold blocks payout, got %d body=%s", payoutRec.Code, payoutRec.Body.String())
	}

	releaseReq := httptest.NewRequest(http.MethodPost, "/v1/ledger/holds/"+holdID+"/release", nil)
	releaseReq.Header.Set("Authorization", authHeader)
	releaseRec := httptest.NewRecorder()
	mux.ServeHTTP(releaseRec, releaseReq)
	if releaseRec.Code != http.StatusOK {
		t.Fatalf("release hold: expected 200, got %d body=%s", releaseRec.Code, releaseRec.Body.String())
	}
	releaseReq = httptest.NewRequest(http.MethodPost, "/v1/ledger/holds/"+holdID+"/release", nil)
	releaseReq.Header.Set("Authorization", authHeader)
	releaseRec = httptest.NewRecorder()
	mux.ServeHTTP(releaseRec, releaseReq)
	if releaseRec.Code != http.StatusOK {
		t.Fatalf("repeat release hold: expected 200, got %d body=%s", releaseRec.Code, releaseRec.Body.String())
	}

	payoutReq = httptest.NewRequest(http.MethodPost, "/v1/settlements/"+sttl.ID+"/payout", bytes.NewReader([]byte(`{"beneficiary_id":"`+beneficiaryID+`"}`)))
	payoutReq.Header.Set("Content-Type", "application/json")
	payoutReq.Header.Set("Authorization", authHeader)
	payoutRec = httptest.NewRecorder()
	mux.ServeHTTP(payoutRec, payoutReq)
	if payoutRec.Code != http.StatusCreated {
		t.Fatalf("expected payout to succeed after hold release, got %d body=%s", payoutRec.Code, payoutRec.Body.String())
	}
}

func TestIntegrationLedgerHoldExtendAndForceExpire(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, _, _ := buildGatewayMux(db)

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Hold Extend Merchant", Email: "hold-extend@test.com", BusinessType: "company",
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
	authHeader := basicAuth(key.KeyID, key.KeySecret)

	initialExpiry := time.Now().Add(10 * time.Minute).Unix()
	createBody, _ := json.Marshal(map[string]any{
		"account_code": "SETTLEMENT_CLEARING",
		"source_type":  "reserve_hold",
		"source_id":    "reserve_123",
		"reason":       "reserve review",
		"currency":     "INR",
		"amount":       2500,
		"expires_at":   initialExpiry,
	})
	createReq := httptest.NewRequest(http.MethodPost, "/v1/ledger/holds", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", authHeader)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create hold: expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	holdID, _ := created["id"].(string)
	if holdID == "" {
		t.Fatal("expected hold id")
	}

	extendedExpiry := time.Now().Add(45 * time.Minute).Unix()
	extendBody, _ := json.Marshal(map[string]any{"expires_at": extendedExpiry})
	extendReq := httptest.NewRequest(http.MethodPost, "/v1/ledger/holds/"+holdID+"/extend", bytes.NewReader(extendBody))
	extendReq.Header.Set("Authorization", authHeader)
	extendReq.Header.Set("Content-Type", "application/json")
	extendRec := httptest.NewRecorder()
	mux.ServeHTTP(extendRec, extendReq)
	if extendRec.Code != http.StatusOK {
		t.Fatalf("extend hold: expected 200, got %d body=%s", extendRec.Code, extendRec.Body.String())
	}

	var expiresAt time.Time
	if err := db.QueryRow(ctx, `SELECT expires_at FROM paygate_ledger.ledger_holds WHERE id = $1`, holdID).Scan(&expiresAt); err != nil {
		t.Fatalf("query extended expires_at: %v", err)
	}
	if expiresAt.Unix() != extendedExpiry {
		t.Fatalf("expected expires_at %d, got %d", extendedExpiry, expiresAt.Unix())
	}

	expireReq := httptest.NewRequest(http.MethodPost, "/v1/ledger/holds/"+holdID+"/expire", nil)
	expireReq.Header.Set("Authorization", authHeader)
	expireRec := httptest.NewRecorder()
	mux.ServeHTTP(expireRec, expireReq)
	if expireRec.Code != http.StatusOK {
		t.Fatalf("expire hold: expected 200, got %d body=%s", expireRec.Code, expireRec.Body.String())
	}

	var status string
	if err := db.QueryRow(ctx, `SELECT status FROM paygate_ledger.ledger_holds WHERE id = $1`, holdID).Scan(&status); err != nil {
		t.Fatalf("query expired hold status: %v", err)
	}
	if status != string(ledger.HoldStatusExpired) {
		t.Fatalf("expected expired hold, got %s", status)
	}

	expireReq = httptest.NewRequest(http.MethodPost, "/v1/ledger/holds/"+holdID+"/expire", nil)
	expireReq.Header.Set("Authorization", authHeader)
	expireRec = httptest.NewRecorder()
	mux.ServeHTTP(expireRec, expireReq)
	if expireRec.Code != http.StatusOK {
		t.Fatalf("repeat expire hold: expected 200, got %d body=%s", expireRec.Code, expireRec.Body.String())
	}
}
