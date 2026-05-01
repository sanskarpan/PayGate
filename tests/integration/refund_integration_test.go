//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
)

func TestIntegrationRefundCapturedPayment(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	ctx := context.Background()

	// Set up merchant and API key.
	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Refund Merchant", Email: "refund@test.com", BusinessType: "company",
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

	// Create order.
	createdOrder, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: createdMerchant.ID, Amount: 9900, Currency: "INR", Receipt: "refund-test",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	// Authorize and capture payment.
	authorized, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID: createdMerchant.ID, OrderID: createdOrder.ID,
		Amount: createdOrder.Amount, Currency: createdOrder.Currency, Method: "card",
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if _, err := paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, authorized.PaymentID, createdOrder.Amount); err != nil {
		t.Fatalf("capture: %v", err)
	}

	t.Run("full refund", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"amount": 9900, "reason": "customer request"})
		req := httptest.NewRequest(http.MethodPost, "/v1/payments/"+authorized.PaymentID+"/refunds", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["status"] != "processed" {
			t.Fatalf("expected status=processed, got %v", resp["status"])
		}
		if resp["amount"] != float64(9900) {
			t.Fatalf("expected amount=9900, got %v", resp["amount"])
		}

		// Verify ledger entries were written.
		var count int
		if err := db.QueryRow(ctx,
			`SELECT COUNT(*) FROM paygate_ledger.ledger_entries WHERE source_id = $1`,
			resp["id"],
		).Scan(&count); err != nil {
			t.Fatalf("query ledger entries: %v", err)
		}
		if count != 2 {
			t.Fatalf("expected 2 ledger entries for refund, got %d", count)
		}
	})

	t.Run("over-refund is rejected", func(t *testing.T) {
		// Create a second payment to try over-refunding.
		createdOrder2, _ := orderSvc.Create(ctx, order.CreateInput{
			MerchantID: createdMerchant.ID, Amount: 5000, Currency: "INR", Receipt: "refund-test-2",
		})
		authorized2, _ := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
			MerchantID: createdMerchant.ID, OrderID: createdOrder2.ID,
			Amount: createdOrder2.Amount, Currency: createdOrder2.Currency, Method: "card",
		})
		_, _ = paymentSvc.CaptureForMerchant(ctx, createdMerchant.ID, authorized2.PaymentID, createdOrder2.Amount)

		// Try to refund more than captured.
		body, _ := json.Marshal(map[string]any{"amount": 9999})
		req := httptest.NewRequest(http.MethodPost, "/v1/payments/"+authorized2.PaymentID+"/refunds", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for over-refund, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("refund of uncaptured payment is rejected", func(t *testing.T) {
		createdOrder3, _ := orderSvc.Create(ctx, order.CreateInput{
			MerchantID: createdMerchant.ID, Amount: 3000, Currency: "INR", Receipt: "refund-test-3",
		})
		authorized3, _ := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
			MerchantID: createdMerchant.ID, OrderID: createdOrder3.ID,
			Amount: createdOrder3.Amount, Currency: createdOrder3.Currency, Method: "card",
		})
		// Do NOT capture — payment is still 'authorized'.
		body, _ := json.Marshal(map[string]any{"amount": 3000})
		req := httptest.NewRequest(http.MethodPost, "/v1/payments/"+authorized3.PaymentID+"/refunds", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for non-captured refund, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("list refunds for payment", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/payments/"+authorized.PaymentID+"/refunds", nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["count"].(float64) < 1 {
			t.Fatalf("expected at least 1 refund, got %v", resp["count"])
		}
	})
}
