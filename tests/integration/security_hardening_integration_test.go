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

	"github.com/sanskarpan/PayGate/internal/auth"
	"github.com/sanskarpan/PayGate/internal/common/middleware"
	"github.com/sanskarpan/PayGate/internal/common/scrubber"
	"github.com/sanskarpan/PayGate/internal/gateway"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
	"github.com/sanskarpan/PayGate/internal/settlement"
	"github.com/sanskarpan/PayGate/internal/webhook"
)

// TestIntegrationMaxBodyRejectsOversizedRequests verifies that the MaxBody
// middleware returns 413 for payloads larger than the configured limit.
func TestIntegrationMaxBodyRejectsOversizedRequests(t *testing.T) {
	handler := middleware.MaxBody(512, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Under-limit: 100 bytes → 200.
	small := bytes.Repeat([]byte("x"), 100)
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(small))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for small body, got %d", rec.Code)
	}

	// Over-limit: 600 bytes → 413 (http.MaxBytesReader writes 413 when body is read).
	// The middleware installs MaxBytesReader; the handler must read the body.
	handler2 := middleware.MaxBody(512, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 600)
		if _, err := r.Body.Read(buf); err != nil {
			// MaxBytesReader returns an error and the stdlib ResponseWriter
			// flushes a 413 automatically in some Go versions; we rely on
			// http.MaxBytesReader's implicit 413 for oversized reads.
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	large := bytes.Repeat([]byte("x"), 600)
	req2 := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(large))
	rec2 := httptest.NewRecorder()
	handler2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for large body, got %d", rec2.Code)
	}
}

// TestIntegrationScrubberRemovesSensitiveFields verifies that the scrubber
// removes card numbers and password fields from arbitrary strings.
func TestIntegrationScrubberRemovesSensitiveFields(t *testing.T) {
	input := `{"password":"hunter2","card_number":"4111111111111111","amount":5000}`
	out := scrubber.Scrub(input)
	if strings.Contains(out, "hunter2") {
		t.Errorf("scrubber left plaintext password in output: %s", out)
	}
	if strings.Contains(out, "4111111111111111") {
		t.Errorf("scrubber left plaintext card number in output: %s", out)
	}
	if !strings.Contains(out, "5000") {
		t.Errorf("scrubber unexpectedly removed non-sensitive field: %s", out)
	}
}

// TestIntegrationWebhookSecretRotationGracePeriod verifies that after rotation:
//   - The new secret is different from the original.
//   - previous_secret in the DB equals the old secret.
//   - previous_secret_expires_at is ~24 hours in the future.
func TestIntegrationWebhookSecretRotationGracePeriod(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Rotation Merchant", Email: uniqueTestEmail(t, "rotation"), BusinessType: "company",
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

	// Create a webhook subscription.
	subBody, _ := json.Marshal(map[string]any{
		"url":    "https://example.com/hook",
		"events": []string{"payment.captured"},
	})
	subReq := httptest.NewRequest(http.MethodPost, "/v1/webhooks", bytes.NewReader(subBody))
	subReq.Header.Set("Authorization", authHdr)
	subReq.Header.Set("Content-Type", "application/json")
	subRec := httptest.NewRecorder()
	mux.ServeHTTP(subRec, subReq)
	if subRec.Code != http.StatusCreated {
		t.Fatalf("create subscription: expected 201, got %d body=%s", subRec.Code, subRec.Body.String())
	}
	var subResp map[string]any
	if err := json.Unmarshal(subRec.Body.Bytes(), &subResp); err != nil {
		t.Fatalf("decode subscription: %v", err)
	}
	subID := subResp["id"].(string)
	originalSecret := subResp["secret"].(string)

	// Rotate the secret.
	rotReq := httptest.NewRequest(http.MethodPost, "/v1/webhooks/"+subID+"/rotate-secret", nil)
	rotReq.Header.Set("Authorization", authHdr)
	rotRec := httptest.NewRecorder()
	mux.ServeHTTP(rotRec, rotReq)
	if rotRec.Code != http.StatusOK {
		t.Fatalf("rotate secret: expected 200, got %d body=%s", rotRec.Code, rotRec.Body.String())
	}
	var rotResp map[string]any
	if err := json.Unmarshal(rotRec.Body.Bytes(), &rotResp); err != nil {
		t.Fatalf("decode rotate response: %v", err)
	}
	newSecret := rotResp["secret"].(string)
	if newSecret == originalSecret {
		t.Fatal("expected new secret to differ from original")
	}

	// Verify grace period columns in the DB.
	var prevSecret string
	var prevExpiresAt time.Time
	if err := db.QueryRow(ctx,
		`SELECT previous_secret, previous_secret_expires_at
		   FROM paygate_webhooks.webhook_subscriptions WHERE id = $1`, subID,
	).Scan(&prevSecret, &prevExpiresAt); err != nil {
		t.Fatalf("query previous_secret: %v", err)
	}
	if prevSecret != originalSecret {
		t.Errorf("expected previous_secret=%q, got %q", originalSecret, prevSecret)
	}
	// Expiry should be roughly 24 hours from now (±5 minutes tolerance).
	expected := time.Now().Add(webhook.RotateSecretGracePeriod)
	if prevExpiresAt.Before(expected.Add(-5*time.Minute)) || prevExpiresAt.After(expected.Add(5*time.Minute)) {
		t.Errorf("previous_secret_expires_at=%v, expected ~%v", prevExpiresAt, expected)
	}
}

func TestIntegrationWebhookRejectsInsecurePublicHTTPURL(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Webhook URL Merchant", Email: uniqueTestEmail(t, "webhook-url"), BusinessType: "company",
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

	body, _ := json.Marshal(map[string]any{
		"url":    "http://example.com/hook",
		"events": []string{"payment.captured"},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks", bytes.NewReader(body))
	req.Header.Set("Authorization", basicAuth(key.KeyID, key.KeySecret))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for insecure webhook URL, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIntegrationGatewayAdminEndpointsRequireAdminScope(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	merchantSvc := merchant.NewService(merchant.NewPostgresRepository(db), merchant.WithSessionSecret("integration-dashboard-secret"))
	authMw := auth.NewMiddleware(merchantSvc)
	scenarioStore := gateway.NewScenarioStore(db)
	adminHandler := gateway.NewAdminHandler(scenarioStore)

	mux := http.NewServeMux()
	adminHandler.RegisterRoutesWithAuth(mux, func(next http.Handler) http.Handler {
		return authMw.RequireScope(merchant.APIKeyScopeAdmin, next)
	})

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Admin Control Merchant", Email: uniqueTestEmail(t, "admin-control"), BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	writeKey, err := merchantSvc.CreateAPIKey(ctx, m.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeWrite,
	})
	if err != nil {
		t.Fatalf("create write api key: %v", err)
	}
	adminKey, err := merchantSvc.CreateAPIKey(ctx, m.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create admin api key: %v", err)
	}

	noAuthReq := httptest.NewRequest(http.MethodGet, "/v1/gateway/scenarios", nil)
	noAuthRec := httptest.NewRecorder()
	mux.ServeHTTP(noAuthRec, noAuthReq)
	if noAuthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", noAuthRec.Code)
	}

	writeReq := httptest.NewRequest(http.MethodGet, "/v1/gateway/scenarios", nil)
	writeReq.Header.Set("Authorization", basicAuth(writeKey.KeyID, writeKey.KeySecret))
	writeRec := httptest.NewRecorder()
	mux.ServeHTTP(writeRec, writeReq)
	if writeRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin key, got %d", writeRec.Code)
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/v1/gateway/scenarios", nil)
	adminReq.Header.Set("Authorization", basicAuth(adminKey.KeyID, adminKey.KeySecret))
	adminRec := httptest.NewRecorder()
	mux.ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin key, got %d body=%s", adminRec.Code, adminRec.Body.String())
	}
}

func TestIntegrationPayoutRejectedWhileSettlementOnHold(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Payout Hold Merchant", Email: uniqueTestEmail(t, "payout-hold"), BusinessType: "company",
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
		MerchantID: m.ID, Amount: 5000, Currency: "INR", Receipt: "payout-hold-order",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	cardTokenID := createCardTokenViaMux(t, mux, authHeader, false)
	authz, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
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
	if _, err := paymentSvc.CaptureForMerchant(ctx, m.ID, authz.PaymentID, o.Amount); err != nil {
		t.Fatalf("capture payment: %v", err)
	}

	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	settlementSvc := settlement.NewService(settlement.NewPostgresRepository(db, ledgerSvc))
	sttl, err := settlementSvc.RunBatch(ctx, m.ID, time.Unix(0, 0), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("run settlement: %v", err)
	}
	if err := settlementSvc.Hold(ctx, m.ID, sttl.ID, "manual review"); err != nil {
		t.Fatalf("hold settlement: %v", err)
	}

	beneficiaryID := createApprovedBeneficiary(t, mux, adminAuthHeader)
	req := httptest.NewRequest(http.MethodPost, "/v1/settlements/"+sttl.ID+"/payout", bytes.NewReader([]byte(`{"beneficiary_id":"`+beneficiaryID+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for payout on held settlement, got %d body=%s", rec.Code, rec.Body.String())
	}
}
