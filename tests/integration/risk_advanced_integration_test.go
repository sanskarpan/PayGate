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
)

func TestIntegrationRiskBlockedByMerchantFraudConfig(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _ := buildRiskMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Risk Config Merchant", Email: uniqueTestEmail(t, "riskconfig"), BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, m.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHdr := basicAuth(key.KeyID, key.KeySecret)

	sendJSON(t, mux, http.MethodPut, "/v1/risk/config", authHdr, map[string]any{
		"ip_velocity_threshold":       20,
		"device_velocity_threshold":   6,
		"merchant_velocity_threshold": 200,
		"amount_spike_factor":         3,
		"review_threshold":            40,
		"block_threshold":             90,
		"blocked_bins":                []string{"411111"},
		"blocked_countries":           []string{},
		"review_on_country_mismatch":  true,
	}, http.StatusOK)

	orderResp := sendJSON(t, mux, http.MethodPost, "/v1/orders", authHdr, map[string]any{
		"amount": 5000, "currency": "INR", "receipt": "risk-blocked-order",
	}, http.StatusCreated)
	tokenID := createCardTokenViaMux(t, mux, authHdr, false)

	body, _ := json.Marshal(map[string]any{
		"order_id":                orderResp["id"],
		"amount":                  5000,
		"currency":                "INR",
		"method":                  "card",
		"payment_method_token_id": tokenID,
		"risk_context": map[string]any{
			"card_bin": "411111",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/authorize", bytes.NewReader(body))
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected blocked payment 422, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIntegrationRiskHoldAssignAndApproveCapture(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _ := buildRiskMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Risk Review Merchant", Email: uniqueTestEmail(t, "riskreview"), BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, m.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHdr := basicAuth(key.KeyID, key.KeySecret)

	sendJSON(t, mux, http.MethodPut, "/v1/risk/config", authHdr, map[string]any{
		"ip_velocity_threshold":       20,
		"device_velocity_threshold":   1,
		"merchant_velocity_threshold": 200,
		"amount_spike_factor":         3,
		"review_threshold":            40,
		"block_threshold":             90,
		"blocked_bins":                []string{},
		"blocked_countries":           []string{},
		"review_on_country_mismatch":  true,
	}, http.StatusOK)

	orderResp := sendJSON(t, mux, http.MethodPost, "/v1/orders", authHdr, map[string]any{
		"amount": 7000, "currency": "INR", "receipt": "risk-hold-order",
	}, http.StatusCreated)
	tokenID := createCardTokenViaMux(t, mux, authHdr, false)

	paymentResp := sendJSON(t, mux, http.MethodPost, "/v1/payments/authorize", authHdr, map[string]any{
		"order_id":                orderResp["id"],
		"amount":                  7000,
		"currency":                "INR",
		"method":                  "card",
		"payment_method_token_id": tokenID,
		"risk_context": map[string]any{
			"device_fingerprint": "device-repeat-1",
		},
	}, http.StatusCreated)
	paymentID := mustString(t, paymentResp, "id")

	eventsResp := sendJSON(t, mux, http.MethodGet, "/v1/risk/events?unresolved=true", authHdr, nil, http.StatusOK)
	items := eventsResp["items"].([]any)
	if len(items) == 0 {
		t.Fatal("expected pending risk event")
	}
	eventID := items[0].(map[string]any)["id"].(string)

	sendJSON(t, mux, http.MethodPost, "/v1/risk/events/"+eventID+"/assign", authHdr, map[string]any{
		"assigned_to": "risk_operator_1",
	}, http.StatusOK)
	reviewResp := sendJSON(t, mux, http.MethodPost, "/v1/risk/events/"+eventID+"/review", authHdr, map[string]any{
		"decision": "approve",
		"notes":    "approved after manual review",
	}, http.StatusOK)
	if got := reviewResp["review_status"]; got != "approved" {
		t.Fatalf("expected approved review, got %v", got)
	}

	getPayment := sendJSON(t, mux, http.MethodGet, "/v1/payments/"+paymentID, authHdr, nil, http.StatusOK)
	if got := getPayment["status"]; got != "captured" {
		t.Fatalf("expected captured payment after review, got %v", got)
	}
}

func TestIntegrationRiskHoldBlockReversesAuthorization(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _ := buildRiskMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Risk Block Review Merchant", Email: uniqueTestEmail(t, "riskblockreview"), BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, m.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHdr := basicAuth(key.KeyID, key.KeySecret)

	sendJSON(t, mux, http.MethodPut, "/v1/risk/config", authHdr, map[string]any{
		"ip_velocity_threshold":       20,
		"device_velocity_threshold":   1,
		"merchant_velocity_threshold": 200,
		"amount_spike_factor":         3,
		"review_threshold":            40,
		"block_threshold":             90,
		"blocked_bins":                []string{},
		"blocked_countries":           []string{},
		"review_on_country_mismatch":  true,
	}, http.StatusOK)

	orderResp := sendJSON(t, mux, http.MethodPost, "/v1/orders", authHdr, map[string]any{
		"amount": 7100, "currency": "INR", "receipt": "risk-block-review-order",
	}, http.StatusCreated)
	tokenID := createCardTokenViaMux(t, mux, authHdr, false)

	paymentResp := sendJSON(t, mux, http.MethodPost, "/v1/payments/authorize", authHdr, map[string]any{
		"order_id":                orderResp["id"],
		"amount":                  7100,
		"currency":                "INR",
		"method":                  "card",
		"payment_method_token_id": tokenID,
		"risk_context": map[string]any{
			"device_fingerprint": "device-repeat-2",
		},
	}, http.StatusCreated)
	paymentID := mustString(t, paymentResp, "id")

	eventsResp := sendJSON(t, mux, http.MethodGet, "/v1/risk/events?unresolved=true", authHdr, nil, http.StatusOK)
	items := eventsResp["items"].([]any)
	if len(items) == 0 {
		t.Fatal("expected pending risk event")
	}
	eventID := items[0].(map[string]any)["id"].(string)

	reviewResp := sendJSON(t, mux, http.MethodPost, "/v1/risk/events/"+eventID+"/review", authHdr, map[string]any{
		"decision": "block",
		"notes":    "blocked after review",
	}, http.StatusOK)
	if got := reviewResp["review_status"]; got != "blocked" {
		t.Fatalf("expected blocked review, got %v", got)
	}

	getPayment := sendJSON(t, mux, http.MethodGet, "/v1/payments/"+paymentID, authHdr, nil, http.StatusOK)
	if got := getPayment["status"]; got != "authorization_reversed" {
		t.Fatalf("expected reversed authorization after block, got %v", got)
	}
}

func TestIntegrationRiskBlockQueuesReserveEscalationAndApproval(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _ := buildRiskMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Reserve Escalation Merchant", Email: uniqueTestEmail(t, "reserve-escalation"), BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, m.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHdr := basicAuth(key.KeyID, key.KeySecret)

	sendJSON(t, mux, http.MethodPut, "/v1/risk/config", authHdr, map[string]any{
		"ip_velocity_threshold":       20,
		"device_velocity_threshold":   6,
		"merchant_velocity_threshold": 200,
		"amount_spike_factor":         3,
		"review_threshold":            40,
		"block_threshold":             90,
		"blocked_bins":                []string{"411111"},
		"blocked_countries":           []string{},
		"review_on_country_mismatch":  true,
	}, http.StatusOK)

	orderResp := sendJSON(t, mux, http.MethodPost, "/v1/orders", authHdr, map[string]any{
		"amount": 9900, "currency": "INR", "receipt": "reserve-escalation-order",
	}, http.StatusCreated)
	tokenID := createCardTokenViaMux(t, mux, authHdr, false)

	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{
		"order_id":                orderResp["id"],
		"amount":                  9900,
		"currency":                "INR",
		"method":                  "card",
		"payment_method_token_id": tokenID,
		"risk_context": map[string]any{
			"card_bin": "411111",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/authorize", bytes.NewReader(body))
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected blocked payment, got %d body=%s", rec.Code, rec.Body.String())
	}

	escalations := sendJSON(t, mux, http.MethodGet, "/v1/merchants/me/reserve-escalations", authHdr, nil, http.StatusOK)
	if int(escalations["count"].(float64)) < 1 {
		t.Fatalf("expected reserve escalation entry, got %#v", escalations)
	}
	item := escalations["items"].([]any)[0].(map[string]any)
	if got := item["status"]; got != "pending" {
		t.Fatalf("expected pending reserve escalation, got %v", got)
	}

	reviewed := sendJSON(t, mux, http.MethodPost, "/v1/merchants/me/reserve-escalations/"+item["id"].(string)+"/review", authHdr, map[string]any{
		"decision": "approved",
		"notes":    "risk team approved additional reserve",
	}, http.StatusOK)
	if got := reviewed["status"]; got != "approved" {
		t.Fatalf("expected approved reserve escalation, got %v", got)
	}

	policy := sendJSON(t, mux, http.MethodGet, "/v1/merchants/me/reserve-policy", authHdr, nil, http.StatusOK)
	if got := policy["policy_type"]; got != "rolling_percentage" {
		t.Fatalf("expected rolling_percentage reserve policy, got %v", got)
	}
	if got := int(policy["percentage_bps"].(float64)); got < 1000 {
		t.Fatalf("expected reserve percentage to be escalated, got %d", got)
	}
}
