//go:build integration

package integration

import (
	"context"
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

func TestIntegrationPayoutCompletesAndWritesLedger(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	ctx := context.Background()

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Payout Merchant", Email: "payout@test.com", BusinessType: "company",
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
