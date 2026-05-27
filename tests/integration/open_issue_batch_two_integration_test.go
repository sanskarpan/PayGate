//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
)

func TestIntegrationGatewayRoutingFailoverPersistsDecision(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	createdMerchant, err := merchantSvc.CreateMerchant(t.Context(), merchant.CreateMerchantInput{
		Name:         "Routing Merchant",
		Email:        uniqueTestEmail(t, "routing"),
		BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(t.Context(), createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode:  merchant.APIKeyModeTest,
		Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHeader := basicAuth(key.KeyID, key.KeySecret)

	sendJSON(t, mux, http.MethodPost, "/v1/gateway/scenarios", authHeader, map[string]any{
		"merchant_id":  createdMerchant.ID,
		"mode":         "decline",
		"decline_code": "CARD_DECLINED",
	}, http.StatusCreated)
	sendJSON(t, mux, http.MethodPost, "/v1/gateway/routing-policies", authHeader, map[string]any{
		"merchant_id":         createdMerchant.ID,
		"method":              "card",
		"primary_provider":    "simulator_primary",
		"fallback_provider":   "simulator_failover",
		"failover_on_decline": true,
		"failover_on_error":   true,
		"cost_weight":         20,
		"success_weight":      80,
	}, http.StatusCreated)

	orderResp := sendJSON(t, mux, http.MethodPost, "/v1/orders", authHeader, map[string]any{
		"amount":   7300,
		"currency": "INR",
		"receipt":  "routing-failover-order",
	}, http.StatusCreated)
	cardTokenID := createCardTokenViaMux(t, mux, authHeader, false)
	paymentResp := sendJSON(t, mux, http.MethodPost, "/v1/payments/authorize", authHeader, map[string]any{
		"order_id":                mustString(t, orderResp, "id"),
		"amount":                  7300,
		"currency":                "INR",
		"method":                  "card",
		"payment_method_token_id": cardTokenID,
	}, http.StatusCreated)

	if got := mustString(t, paymentResp, "provider"); got != "simulator_failover" {
		t.Fatalf("expected failover provider, got %s", got)
	}
	if got := mustString(t, paymentResp, "routing_reason"); !strings.Contains(got, "failover") {
		t.Fatalf("expected failover routing reason, got %s", got)
	}
	attempted, ok := paymentResp["attempted_providers"].([]any)
	if !ok || len(attempted) != 2 {
		t.Fatalf("expected two attempted providers, got %#v", paymentResp["attempted_providers"])
	}
	if attempted[0] != "simulator_primary" || attempted[1] != "simulator_failover" {
		t.Fatalf("unexpected attempted providers %#v", attempted)
	}
}

func TestIntegrationNetbankingRedirectLifecycle(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, orderSvc, _ := buildGatewayMux(db)
	createdMerchant, err := merchantSvc.CreateMerchant(t.Context(), merchant.CreateMerchantInput{
		Name:         "Netbanking Merchant",
		Email:        uniqueTestEmail(t, "netbanking"),
		BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(t.Context(), createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode:  merchant.APIKeyModeTest,
		Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHeader := basicAuth(key.KeyID, key.KeySecret)

	orderModel, err := orderSvc.Create(t.Context(), order.CreateInput{
		MerchantID: createdMerchant.ID,
		Amount:     8400,
		Currency:   "INR",
		Receipt:    "netbanking-order",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	redirectResp := sendJSON(t, mux, http.MethodPost, "/v1/payments/netbanking/redirects", authHeader, map[string]any{
		"order_id":  orderModel.ID,
		"amount":    orderModel.Amount,
		"currency":  orderModel.Currency,
		"bank_code": "HDFC",
	}, http.StatusCreated)
	if got := mustString(t, redirectResp, "method"); got != "netbanking" {
		t.Fatalf("expected netbanking method, got %s", got)
	}
	if got := mustString(t, redirectResp, "status"); got != "pending_customer_action" {
		t.Fatalf("expected pending_customer_action, got %s", got)
	}
	nextAction := mustMap(t, redirectResp, "next_action")
	if mustString(t, nextAction, "type") != "netbanking_redirect" {
		t.Fatalf("unexpected next action %#v", nextAction)
	}
	sandbox := mustMap(t, redirectResp, "sandbox")
	callbackURL := mustString(t, sandbox, "callback_url")

	callbackPayload := map[string]any{
		"merchant_id":       createdMerchant.ID,
		"event_id":          "nbk_evt_1",
		"status":            "succeeded",
		"gateway_reference": "nbk_success_1",
	}
	callbackResp := sendJSON(t, mux, http.MethodPost, strings.TrimPrefix(callbackURL, "http://example.com"), "", callbackPayload, http.StatusAccepted)
	if processed, ok := callbackResp["processed"].(bool); !ok || !processed {
		t.Fatalf("expected callback processed=true, got %#v", callbackResp)
	}
	duplicateResp := sendJSON(t, mux, http.MethodPost, strings.TrimPrefix(callbackURL, "http://example.com"), "", callbackPayload, http.StatusAccepted)
	if processed, ok := duplicateResp["processed"].(bool); !ok || processed {
		t.Fatalf("expected duplicate callback processed=false, got %#v", duplicateResp)
	}

	getResp := sendJSON(t, mux, http.MethodGet, fmt.Sprintf("/v1/payments/%s/redirect-session", mustString(t, redirectResp, "id")), authHeader, nil, http.StatusOK)
	if got := mustString(t, getResp, "status"); got != "captured" {
		t.Fatalf("expected captured redirect payment, got %s", got)
	}
	if got := mustString(t, getResp, "provider_status"); got != "succeeded" {
		t.Fatalf("expected succeeded provider status, got %s", got)
	}
}

func TestIntegrationWalletRedirectTimeoutAndFailure(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, orderSvc, _ := buildGatewayMux(db)
	createdMerchant, err := merchantSvc.CreateMerchant(t.Context(), merchant.CreateMerchantInput{
		Name:         "Wallet Merchant",
		Email:        uniqueTestEmail(t, "wallet"),
		BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(t.Context(), createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode:  merchant.APIKeyModeTest,
		Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHeader := basicAuth(key.KeyID, key.KeySecret)

	sendJSON(t, mux, http.MethodPost, "/v1/gateway/scenarios", authHeader, map[string]any{
		"merchant_id": createdMerchant.ID,
		"mode":        "timeout",
	}, http.StatusCreated)
	orderModel, err := orderSvc.Create(t.Context(), order.CreateInput{
		MerchantID: createdMerchant.ID,
		Amount:     3900,
		Currency:   "INR",
		Receipt:    "wallet-order",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	redirectResp := sendJSON(t, mux, http.MethodPost, "/v1/payments/wallet/redirects", authHeader, map[string]any{
		"order_id":           orderModel.ID,
		"amount":             orderModel.Amount,
		"currency":           orderModel.Currency,
		"wallet_code":        "paytm",
		"expires_in_seconds": 1,
	}, http.StatusCreated)
	paymentID := mustString(t, redirectResp, "id")
	pollResp := sendJSON(t, mux, http.MethodPost, fmt.Sprintf("/v1/payments/%s/redirect-session/poll", paymentID), authHeader, map[string]any{}, http.StatusOK)
	if got := mustString(t, pollResp, "status"); got != "processing" && got != "pending_customer_action" {
		t.Fatalf("expected processing or pending state, got %s", got)
	}
	if _, err := db.Exec(t.Context(), `UPDATE paygate_payments.redirect_payment_details SET expires_at = NOW() - INTERVAL '1 second' WHERE payment_id = $1`, paymentID); err != nil {
		t.Fatalf("expire redirect session: %v", err)
	}
	expiredResp := sendJSON(t, mux, http.MethodPost, fmt.Sprintf("/v1/payments/%s/redirect-session/poll", paymentID), authHeader, map[string]any{}, http.StatusOK)
	if got := mustString(t, expiredResp, "provider_status"); got != "expired" {
		t.Fatalf("expected expired provider status, got %s", got)
	}
	if got := mustString(t, expiredResp, "status"); got != "failed" {
		t.Fatalf("expected failed payment state after expiry, got %s", got)
	}
}

func mustMap(t *testing.T, payload map[string]any, key string) map[string]any {
	t.Helper()
	raw, ok := payload[key]
	if !ok {
		t.Fatalf("missing key %q in %#v", key, payload)
	}
	out, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected object at key %q, got %#v", key, raw)
	}
	return out
}
