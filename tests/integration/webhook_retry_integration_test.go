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

	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/webhook"
)

func TestIntegrationWebhookSecretRotation(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Rotate Merchant", Email: "rotate@test.com", BusinessType: "company",
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

	// Create a subscription.
	body, _ := json.Marshal(map[string]any{
		"url":    "https://example.com/webhook",
		"events": []string{"payment.captured"},
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
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	webhookID := created["id"].(string)
	originalSecret := created["secret"].(string)

	// Rotate the secret.
	rotateReq := httptest.NewRequest(http.MethodPost, "/v1/webhooks/"+webhookID+"/rotate-secret", nil)
	rotateReq.Header.Set("Authorization", authHeader)
	rotateRec := httptest.NewRecorder()
	mux.ServeHTTP(rotateRec, rotateReq)
	if rotateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on rotate, got %d body=%s", rotateRec.Code, rotateRec.Body.String())
	}
	var rotated map[string]any
	_ = json.Unmarshal(rotateRec.Body.Bytes(), &rotated)

	newSecret, ok := rotated["secret"].(string)
	if !ok || newSecret == "" {
		t.Fatal("expected new secret in rotate response")
	}
	if newSecret == originalSecret {
		t.Fatal("expected new secret to differ from original")
	}
}

func TestIntegrationWebhookReplay(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	createdMerchant, err := merchant.NewService(
		merchant.NewPostgresRepository(db),
	).CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Replay Webhook Merchant", Email: "replay-wh@test.com", BusinessType: "company",
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

	payload := map[string]any{"event_type": "payment.captured", "payment_id": "pay_replay_test"}
	const eventID = "evt_replay_test"

	// First delivery.
	if err := webhookSvc.DeliverEvent(ctx, eventID, createdMerchant.ID, "payment.captured", payload); err != nil {
		t.Fatalf("first deliver: %v", err)
	}
	if deliveryCount != 1 {
		t.Fatalf("expected 1 delivery after first deliver, got %d", deliveryCount)
	}

	// Replay: should re-deliver (new replay ID bypasses idempotency).
	n, err := webhookSvc.ReplayEvent(ctx, createdMerchant.ID, eventID)
	if err != nil {
		t.Fatalf("replay event: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 replayed delivery, got %d", n)
	}
	if deliveryCount != 2 {
		t.Fatalf("expected 2 total deliveries after replay, got %d", deliveryCount)
	}
}

func TestIntegrationWebhookRetryWorker(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	createdMerchant, err := merchant.NewService(
		merchant.NewPostgresRepository(db),
	).CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Retry Worker Merchant", Email: "retry-worker@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}

	// Start a server that fails on the first call and succeeds on subsequent.
	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	webhookSvc := webhook.NewService(webhook.NewPostgresRepository(db))
	sub, err := webhookSvc.CreateSubscription(ctx, webhook.CreateInput{
		MerchantID: createdMerchant.ID,
		URL:        mockServer.URL,
		Events:     []string{"payment.captured"},
	})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	payload := map[string]any{"event_type": "payment.captured"}
	if err := webhookSvc.DeliverEvent(ctx, "evt_retry_worker_test", createdMerchant.ID, "payment.captured", payload); err != nil {
		t.Fatalf("deliver event: %v", err)
	}

	// First delivery should have failed.
	attempts, err := webhookSvc.ListDeliveryAttempts(ctx, createdMerchant.ID, sub.ID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	if len(attempts) != 1 || attempts[0].Status != webhook.DeliveryFailed {
		t.Fatalf("expected 1 failed attempt, got %d attempts, status=%v", len(attempts), func() webhook.DeliveryStatus {
			if len(attempts) > 0 {
				return attempts[0].Status
			}
			return ""
		}())
	}

	// Backdate the next_retry_at so the retry worker picks it up immediately.
	if _, err := db.Exec(ctx,
		`UPDATE paygate_webhooks.webhook_delivery_attempts SET next_retry_at = NOW() - INTERVAL '1 minute' WHERE id = $1`,
		attempts[0].ID,
	); err != nil {
		t.Fatalf("backdate next_retry_at: %v", err)
	}

	// Run the retry worker once.
	n, err := webhookSvc.RetryPendingDeliveries(ctx, 10)
	if err != nil {
		t.Fatalf("retry pending: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 retried delivery, got %d", n)
	}

	// Wait a brief moment for the second delivery to be recorded.
	time.Sleep(10 * time.Millisecond)

	// Re-fetch attempts: the original should now be succeeded.
	attempts, err = webhookSvc.ListDeliveryAttempts(ctx, createdMerchant.ID, sub.ID)
	if err != nil {
		t.Fatalf("re-list attempts: %v", err)
	}
	// The UpdateDeliveryAttempt updates the original record to succeeded.
	var succeeded bool
	for _, a := range attempts {
		if a.Status == webhook.DeliverySucceeded {
			succeeded = true
			break
		}
	}
	if !succeeded {
		t.Fatalf("expected at least one succeeded delivery after retry, got statuses: %v", func() []webhook.DeliveryStatus {
			var ss []webhook.DeliveryStatus
			for _, a := range attempts {
				ss = append(ss, a.Status)
			}
			return ss
		}())
	}
}
