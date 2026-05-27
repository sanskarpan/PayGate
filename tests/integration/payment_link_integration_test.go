//go:build integration

package integration

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanskarpan/PayGate/internal/merchant"
)

func TestIntegrationPaymentLinkLifecycleAndPublicResolve(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	createdMerchant, err := merchantSvc.CreateMerchant(t.Context(), merchant.CreateMerchantInput{
		Name:         "Payment Link Merchant",
		Email:        uniqueTestEmail(t, "payment-link"),
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

	linkResp := sendJSON(t, mux, http.MethodPost, "/v1/payment-links", authHeader, map[string]any{
		"title":              "June Collection",
		"description":        "June invoice payment",
		"external_reference": "plink-june",
		"amount":             6600,
		"currency":           "INR",
		"callback_url":       "/checkout/callback",
		"notes": map[string]any{
			"source": "integration",
		},
	}, http.StatusCreated)
	linkID := mustString(t, linkResp, "id")
	if got := mustString(t, linkResp, "status"); got != "active" {
		t.Fatalf("expected active payment link, got %s", got)
	}
	if got := mustString(t, linkResp, "public_url"); !strings.Contains(got, "/pay/") {
		t.Fatalf("expected public url, got %s", got)
	}

	getResp := sendJSON(t, mux, http.MethodGet, "/v1/payment-links/"+linkID, authHeader, nil, http.StatusOK)
	if mustString(t, getResp, "order_id") == "" {
		t.Fatalf("expected order_id on payment link, got %#v", getResp)
	}

	publicReq := httptest.NewRequest(http.MethodGet, "/pay/"+linkID+"?merchant_id="+createdMerchant.ID, nil)
	publicRecorder := httptest.NewRecorder()
	mux.ServeHTTP(publicRecorder, publicReq)
	if publicRecorder.Code != http.StatusFound {
		t.Fatalf("expected public resolve redirect, got %d body=%s", publicRecorder.Code, publicRecorder.Body.String())
	}
	location := publicRecorder.Header().Get("Location")
	if !strings.Contains(location, "/checkout?") || !strings.Contains(location, "order_id="+mustString(t, linkResp, "order_id")) {
		t.Fatalf("unexpected payment link redirect location %s", location)
	}

	if _, err := db.Exec(t.Context(), `UPDATE paygate_orders.orders SET status = 'paid', amount_due = 0, amount_paid = amount WHERE id = $1`, mustString(t, linkResp, "order_id")); err != nil {
		t.Fatalf("mark order paid: %v", err)
	}
	paidResp := sendJSON(t, mux, http.MethodGet, "/v1/payment-links/"+linkID, authHeader, nil, http.StatusOK)
	if got := mustString(t, paidResp, "status"); got != "paid" {
		t.Fatalf("expected paid payment link, got %s", got)
	}

	secondLink := sendJSON(t, mux, http.MethodPost, "/v1/payment-links", authHeader, map[string]any{
		"title":        "Disable Me",
		"amount":       1200,
		"currency":     "INR",
		"callback_url": "/checkout/callback",
	}, http.StatusCreated)
	secondID := mustString(t, secondLink, "id")
	sendJSON(t, mux, http.MethodPost, "/v1/payment-links/"+secondID+"/disable", authHeader, map[string]any{}, http.StatusOK)
	disabledReq := httptest.NewRequest(http.MethodGet, "/pay/"+secondID+"?merchant_id="+createdMerchant.ID, nil)
	disabledRecorder := httptest.NewRecorder()
	mux.ServeHTTP(disabledRecorder, disabledReq)
	if disabledRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected disabled payment link resolve failure, got %d body=%s", disabledRecorder.Code, disabledRecorder.Body.String())
	}
}
