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
	"github.com/sanskarpan/PayGate/internal/webhook"
)

func TestIntegrationWebhookSubscriptionCRUD(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Webhook Merchant", Email: "webhook@test.com", BusinessType: "company",
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

	// Create a webhook subscription.
	body, _ := json.Marshal(map[string]any{
		"url":    "https://example.com/webhook",
		"events": []string{"payment.captured", "refund.*"},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created["id"] == nil || created["id"] == "" {
		t.Fatal("expected non-empty id")
	}
	if created["secret"] == nil || created["secret"] == "" {
		t.Fatal("expected secret on creation response")
	}
	if created["status"] != "active" {
		t.Fatalf("expected status=active, got %v", created["status"])
	}

	webhookID := created["id"].(string)

	t.Run("get subscription", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/webhooks/"+webhookID, nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		// Secret must NOT be returned on GET.
		if resp["secret"] != nil {
			t.Fatalf("secret must not be returned on GET, got %v", resp["secret"])
		}
	})

	t.Run("list subscriptions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/webhooks", nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["count"].(float64) < 1 {
			t.Fatalf("expected at least 1 subscription, got %v", resp["count"])
		}
	})

	t.Run("disable subscription", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/"+webhookID+"/disable", nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["status"] != "disabled" {
			t.Fatalf("expected status=disabled, got %v", resp["status"])
		}
	})

	t.Run("re-enable subscription", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/"+webhookID+"/enable", nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["status"] != "active" {
			t.Fatalf("expected status=active, got %v", resp["status"])
		}
	})

	t.Run("delete subscription", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/v1/webhooks/"+webhookID, nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("get deleted subscription returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/webhooks/"+webhookID, nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestIntegrationWebhookDelivery(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	_, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)

	// Set up a mock HTTP server to receive the webhook.
	received := make(chan []byte, 10)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Delivery Merchant", Email: "delivery@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}

	// Create webhook subscription pointing at mock server.
	webhookRepo := webhook.NewPostgresRepository(db)
	webhookSvc := webhook.NewService(webhookRepo)
	sub, err := webhookSvc.CreateSubscription(ctx, webhook.CreateInput{
		MerchantID: createdMerchant.ID,
		URL:        mockServer.URL,
		Events:     []string{"payment.captured"},
	})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	// Create an order + capture a payment.
	createdOrder, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: createdMerchant.ID, Amount: 5000, Currency: "INR", Receipt: "wh-test",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
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

	// Deliver a payment.captured event via the webhook service.
	payload := map[string]any{
		"event_type": "payment.captured",
		"payment_id": authorized.PaymentID,
		"amount":     createdOrder.Amount,
		"currency":   createdOrder.Currency,
	}
	if err := webhookSvc.DeliverEvent(ctx, "evt_test_delivery", createdMerchant.ID, "payment.captured", payload); err != nil {
		t.Fatalf("deliver event: %v", err)
	}

	// Verify the mock server received the POST.
	select {
	case body := <-received:
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode received webhook: %v", err)
		}
		if got["event_type"] != "payment.captured" {
			t.Fatalf("expected event_type=payment.captured, got %v", got["event_type"])
		}
	default:
		t.Fatal("expected webhook to be delivered to mock server")
	}

	// Verify delivery attempt was recorded in DB.
	attempts, err := webhookSvc.ListDeliveryAttempts(ctx, createdMerchant.ID, sub.ID)
	if err != nil {
		t.Fatalf("list delivery attempts: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("expected 1 delivery attempt, got %d", len(attempts))
	}
	if attempts[0].Status != webhook.DeliverySucceeded {
		t.Fatalf("expected succeeded delivery, got %s", attempts[0].Status)
	}
}

func TestIntegrationWebhookIdempotentDelivery(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	createdMerchant, err := merchant.NewService(
		merchant.NewPostgresRepository(db),
	).CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Idem Webhook Merchant", Email: "idem-wh@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}

	deliveryCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deliveryCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	webhookSvc := webhook.NewService(webhook.NewPostgresRepository(db))
	if _, err := webhookSvc.CreateSubscription(ctx, webhook.CreateInput{
		MerchantID: createdMerchant.ID,
		URL:        mockServer.URL,
		Events:     []string{"payment.captured"},
	}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	payload := map[string]any{"event_type": "payment.captured"}
	const eventID = "evt_idempotent_test"

	// Deliver the same event twice.
	if err := webhookSvc.DeliverEvent(ctx, eventID, createdMerchant.ID, "payment.captured", payload); err != nil {
		t.Fatalf("first deliver: %v", err)
	}
	if err := webhookSvc.DeliverEvent(ctx, eventID, createdMerchant.ID, "payment.captured", payload); err != nil {
		t.Fatalf("second deliver: %v", err)
	}

	// Mock server should have been called exactly once.
	if deliveryCount != 1 {
		t.Fatalf("expected 1 delivery (idempotent), got %d", deliveryCount)
	}
}
