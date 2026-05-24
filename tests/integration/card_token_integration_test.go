//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/sanskarpan/PayGate/internal/merchant"
)

func TestIntegrationCardTokenLifecycleAndAuthorization(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)

	createdMerchant, err := merchantSvc.CreateMerchant(t.Context(), merchant.CreateMerchantInput{
		Name:         "Card Token Merchant",
		Email:        uniqueTestEmail(t, "card-token"),
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

	singleUseTokenID := createCardTokenViaMux(t, mux, authHeader, false)
	orderResp := sendJSON(t, mux, http.MethodPost, "/v1/orders", authHeader, map[string]any{
		"amount":   4200,
		"currency": "INR",
		"receipt":  "card-token-single-use",
	}, http.StatusCreated)
	paymentResp := sendJSON(t, mux, http.MethodPost, "/v1/payments/authorize", authHeader, map[string]any{
		"order_id":                mustString(t, orderResp, "id"),
		"amount":                  4200,
		"currency":                "INR",
		"method":                  "card",
		"payment_method_token_id": singleUseTokenID,
	}, http.StatusCreated)
	if got := mustString(t, paymentResp, "payment_method_token_id"); got != singleUseTokenID {
		t.Fatalf("expected payment token id %s, got %s", singleUseTokenID, got)
	}
	card, ok := paymentResp["card"].(map[string]any)
	if !ok {
		t.Fatalf("expected card metadata, got %#v", paymentResp["card"])
	}
	if last4, _ := card["last4"].(string); last4 != "1111" {
		t.Fatalf("expected last4 1111, got %#v", card["last4"])
	}

	secondOrderResp := sendJSON(t, mux, http.MethodPost, "/v1/orders", authHeader, map[string]any{
		"amount":   4200,
		"currency": "INR",
		"receipt":  "card-token-single-use-second",
	}, http.StatusCreated)
	sendJSON(t, mux, http.MethodPost, "/v1/payments/authorize", authHeader, map[string]any{
		"order_id":                mustString(t, secondOrderResp, "id"),
		"amount":                  4200,
		"currency":                "INR",
		"method":                  "card",
		"payment_method_token_id": singleUseTokenID,
	}, http.StatusBadRequest)

	reusableTokenID := createCardTokenViaMux(t, mux, authHeader, true)
	firstReusableOrder := sendJSON(t, mux, http.MethodPost, "/v1/orders", authHeader, map[string]any{
		"amount":   5100,
		"currency": "INR",
		"receipt":  "card-token-reusable-1",
	}, http.StatusCreated)
	sendJSON(t, mux, http.MethodPost, "/v1/payments/authorize", authHeader, map[string]any{
		"order_id":                mustString(t, firstReusableOrder, "id"),
		"amount":                  5100,
		"currency":                "INR",
		"method":                  "card",
		"payment_method_token_id": reusableTokenID,
	}, http.StatusCreated)
	secondReusableOrder := sendJSON(t, mux, http.MethodPost, "/v1/orders", authHeader, map[string]any{
		"amount":   5200,
		"currency": "INR",
		"receipt":  "card-token-reusable-2",
	}, http.StatusCreated)
	sendJSON(t, mux, http.MethodPost, "/v1/payments/authorize", authHeader, map[string]any{
		"order_id":                mustString(t, secondReusableOrder, "id"),
		"amount":                  5200,
		"currency":                "INR",
		"method":                  "card",
		"payment_method_token_id": reusableTokenID,
	}, http.StatusCreated)

	sendJSON(t, mux, http.MethodPost, "/v1/card-tokens/"+reusableTokenID+"/disable", authHeader, map[string]any{
		"reason": "customer_request",
	}, http.StatusOK)

	thirdReusableOrder := sendJSON(t, mux, http.MethodPost, "/v1/orders", authHeader, map[string]any{
		"amount":   5300,
		"currency": "INR",
		"receipt":  "card-token-reusable-3",
	}, http.StatusCreated)
	sendJSON(t, mux, http.MethodPost, "/v1/payments/authorize", authHeader, map[string]any{
		"order_id":                mustString(t, thirdReusableOrder, "id"),
		"amount":                  5300,
		"currency":                "INR",
		"method":                  "card",
		"payment_method_token_id": reusableTokenID,
	}, http.StatusBadRequest)
}
