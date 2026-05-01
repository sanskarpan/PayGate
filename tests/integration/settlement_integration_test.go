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

func TestIntegrationSettlementBatch(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	ctx := context.Background()

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Settlement Merchant", Email: "settlement@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeRead,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHeader := basicAuth(key.KeyID, key.KeySecret)

	// Create and capture two payments.
	capturePayment := func(amount int64, receipt string) {
		t.Helper()
		o, err := orderSvc.Create(ctx, order.CreateInput{
			MerchantID: createdMerchant.ID, Amount: amount, Currency: "INR", Receipt: receipt,
		})
		if err != nil {
			t.Fatalf("create order: %v", err)
		}
		auth, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
			MerchantID: createdMerchant.ID, OrderID: o.ID,
			Amount: o.Amount, Currency: o.Currency, Method: "card",
		})
		if err != nil {
			t.Fatalf("authorize: %v", err)
		}
		if _, err := paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, auth.PaymentID, o.Amount); err != nil {
			t.Fatalf("capture: %v", err)
		}
	}
	capturePayment(10000, "settle-1")
	capturePayment(5000, "settle-2")

	// Run settlement batch covering from epoch to now+1h.
	settlementSvc := settlement.NewService(settlement.NewPostgresRepository(db, ledger.NewService(ledger.NewRepository(db))))
	periodEnd := time.Now().Add(time.Hour)
	sttl, err := settlementSvc.RunBatch(ctx, createdMerchant.ID, time.Unix(0, 0), periodEnd)
	if err != nil {
		t.Fatalf("run batch: %v", err)
	}

	if sttl.PaymentCount != 2 {
		t.Fatalf("expected 2 payments, got %d", sttl.PaymentCount)
	}
	// Total: 10000 + 5000 = 15000; fees: 200 + 100 = 300; net: 14700
	if sttl.TotalAmount != 15000 {
		t.Fatalf("expected total_amount=15000, got %d", sttl.TotalAmount)
	}
	if sttl.TotalFees != 300 {
		t.Fatalf("expected total_fees=300, got %d", sttl.TotalFees)
	}
	if sttl.NetAmount != 14700 {
		t.Fatalf("expected net_amount=14700, got %d", sttl.NetAmount)
	}
	if sttl.Status != settlement.StateProcessed {
		t.Fatalf("expected status=processed, got %s", sttl.Status)
	}

	// Verify payments are marked settled.
	var unsettledCount int
	if err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM paygate_payments.payments WHERE merchant_id = $1 AND settled = false AND status = 'captured'`,
		createdMerchant.ID,
	).Scan(&unsettledCount); err != nil {
		t.Fatalf("query unsettled: %v", err)
	}
	if unsettledCount != 0 {
		t.Fatalf("expected 0 unsettled payments after batch, got %d", unsettledCount)
	}

	// Verify ledger entries were written (Dr. MERCHANT_PAYABLE / Cr. SETTLEMENT_CLEARING).
	var count int
	if err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM paygate_ledger.ledger_entries WHERE source_id = $1`,
		sttl.ID,
	).Scan(&count); err != nil {
		t.Fatalf("query ledger entries: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 ledger entries for settlement, got %d", count)
	}

	// Verify outbox event was written.
	var evCount int
	if err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM public.outbox WHERE aggregate_id = $1 AND event_type = 'settlement.processed'`,
		sttl.ID,
	).Scan(&evCount); err != nil {
		t.Fatalf("query outbox event: %v", err)
	}
	if evCount != 1 {
		t.Fatalf("expected 1 settlement.processed outbox event, got %d", evCount)
	}

	t.Run("list settlements via API", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/settlements", nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("get settlement detail via API", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/settlements/"+sttl.ID, nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("second batch with no eligible payments returns error", func(t *testing.T) {
		_, err := settlementSvc.RunBatch(ctx, createdMerchant.ID, time.Unix(0, 0), periodEnd)
		if err == nil {
			t.Fatal("expected error for empty batch")
		}
	})
}
