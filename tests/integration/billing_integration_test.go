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
)

func TestIntegrationSubscriptionRecurringChargeFlow(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Billing Merchant", Email: "billing@test.com", BusinessType: "company",
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

	customerResp := sendJSON(t, mux, http.MethodPost, "/v1/customers", authHdr, map[string]any{
		"name":               "Recurring Customer",
		"email":              "customer@test.com",
		"external_reference": "cust-ext-1",
	}, http.StatusCreated)
	customerID := mustString(t, customerResp, "id")

	expires := time.Now().UTC().AddDate(3, 0, 0)
	tokenBody, _ := json.Marshal(map[string]any{
		"card_number":  "4111111111111111",
		"exp_month":    int(expires.Month()),
		"exp_year":     expires.Year(),
		"reusable":     true,
		"customer_ref": customerID,
	})
	tokenReq := httptest.NewRequest(http.MethodPost, "/v1/card-tokens", bytes.NewReader(tokenBody))
	tokenReq.Header.Set("Authorization", authHdr)
	tokenReq.Header.Set("Content-Type", "application/json")
	tokenRec := httptest.NewRecorder()
	mux.ServeHTTP(tokenRec, tokenReq)
	if tokenRec.Code != http.StatusCreated {
		t.Fatalf("create reusable card token: expected 201 got %d body=%s", tokenRec.Code, tokenRec.Body.String())
	}
	var tokenResp map[string]any
	if err := json.Unmarshal(tokenRec.Body.Bytes(), &tokenResp); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	tokenID := mustString(t, tokenResp, "id")

	subscriptionResp := sendJSON(t, mux, http.MethodPost, "/v1/subscriptions", authHdr, map[string]any{
		"customer_id":             customerID,
		"plan_name":               "Growth Monthly",
		"payment_method_token_id": tokenID,
		"amount":                  49900,
		"currency":                "INR",
		"interval_unit":           "month",
		"interval_count":          1,
		"starts_at":               time.Now().UTC().Add(-time.Minute).Unix(),
		"max_retry_count":         2,
		"retry_interval_hours":    2,
	}, http.StatusCreated)
	subscriptionID := mustString(t, subscriptionResp, "id")

	runResp := sendJSON(t, mux, http.MethodPost, "/v1/subscriptions/"+subscriptionID+"/run", authHdr, map[string]any{}, http.StatusOK)
	invoiceMap := runResp["invoice"].(map[string]any)
	if got := invoiceMap["status"]; got != "paid" {
		t.Fatalf("expected paid invoice, got %v", got)
	}
	if invoiceMap["payment_id"] == "" || invoiceMap["order_id"] == "" {
		t.Fatalf("expected invoice payment/order ids, got %#v", invoiceMap)
	}
	paymentMap := runResp["payment"].(map[string]any)
	if paymentMap["captured"] != true {
		t.Fatalf("expected captured subscription payment, got %#v", paymentMap)
	}

	listResp := sendJSON(t, mux, http.MethodGet, "/v1/invoices?subscription_id="+subscriptionID, authHdr, nil, http.StatusOK)
	if int(listResp["count"].(float64)) < 1 {
		t.Fatalf("expected invoice list to include created invoice, got %#v", listResp)
	}
}

func TestIntegrationSubscriptionPauseResumeCancel(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Billing Lifecycle", Email: "billinglifecycle@test.com", BusinessType: "company",
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

	customerResp := sendJSON(t, mux, http.MethodPost, "/v1/customers", authHdr, map[string]any{"name": "Lifecycle Customer"}, http.StatusCreated)
	customerID := mustString(t, customerResp, "id")
	tokenID := createCardTokenViaMux(t, mux, authHdr, true)
	subscriptionResp := sendJSON(t, mux, http.MethodPost, "/v1/subscriptions", authHdr, map[string]any{
		"customer_id":             customerID,
		"plan_name":               "Starter",
		"payment_method_token_id": tokenID,
		"amount":                  9900,
		"currency":                "INR",
		"interval_unit":           "month",
		"interval_count":          1,
		"starts_at":               time.Now().UTC().Add(time.Hour).Unix(),
	}, http.StatusCreated)
	subscriptionID := mustString(t, subscriptionResp, "id")

	pauseResp := sendJSON(t, mux, http.MethodPost, "/v1/subscriptions/"+subscriptionID+"/pause", authHdr, map[string]any{"reason": "merchant requested"}, http.StatusOK)
	if got := pauseResp["status"]; got != "paused" {
		t.Fatalf("expected paused subscription, got %v", got)
	}

	resumeResp := sendJSON(t, mux, http.MethodPost, "/v1/subscriptions/"+subscriptionID+"/resume", authHdr, map[string]any{}, http.StatusOK)
	if got := resumeResp["status"]; got != "active" {
		t.Fatalf("expected active subscription after resume, got %v", got)
	}

	cancelResp := sendJSON(t, mux, http.MethodPost, "/v1/subscriptions/"+subscriptionID+"/cancel", authHdr, map[string]any{"at_period_end": false}, http.StatusOK)
	if got := cancelResp["status"]; got != "canceled" {
		t.Fatalf("expected canceled subscription, got %v", got)
	}
}

func TestIntegrationVirtualAccountMatchAndReviewQueue(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Collection Merchant", Email: "collections@test.com", BusinessType: "company",
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

	customerResp := sendJSON(t, mux, http.MethodPost, "/v1/customers", authHdr, map[string]any{"name": "Collections Customer"}, http.StatusCreated)
	customerID := mustString(t, customerResp, "id")
	orderResp := sendJSON(t, mux, http.MethodPost, "/v1/orders", authHdr, map[string]any{
		"amount": 8800, "currency": "INR", "receipt": "smart-collect-order",
	}, http.StatusCreated)
	orderID := mustString(t, orderResp, "id")

	virtualAccountResp := sendJSON(t, mux, http.MethodPost, "/v1/virtual-accounts", authHdr, map[string]any{
		"customer_id": customerID,
		"order_id":    orderID,
		"reference":   "enterprise-invoice-1",
	}, http.StatusCreated)
	virtualAccountID := mustString(t, virtualAccountResp, "id")

	matchedResp := sendJSON(t, mux, http.MethodPost, "/v1/virtual-accounts/inbound", authHdr, map[string]any{
		"virtual_account_id": virtualAccountID,
		"amount":             8800,
		"currency":           "INR",
		"utr":                "UTR-1001",
		"remitter_name":      "Acme Accounts",
	}, http.StatusCreated)
	if got := matchedResp["status"]; got != "matched" {
		t.Fatalf("expected auto-matched collection, got %v", got)
	}
	if got := matchedResp["order_id"]; got != orderID {
		t.Fatalf("expected matched order_id %s, got %v", orderID, got)
	}

	unmatchedVAResp := sendJSON(t, mux, http.MethodPost, "/v1/virtual-accounts", authHdr, map[string]any{
		"customer_id": customerID,
		"reference":   "enterprise-invoice-2",
	}, http.StatusCreated)
	unmatchedVAID := mustString(t, unmatchedVAResp, "id")
	reviewResp := sendJSON(t, mux, http.MethodPost, "/v1/virtual-accounts/inbound", authHdr, map[string]any{
		"virtual_account_id": unmatchedVAID,
		"amount":             9200,
		"currency":           "INR",
		"utr":                "UTR-1002",
		"remitter_name":      "Loose Deposit",
	}, http.StatusCreated)
	if got := reviewResp["status"]; got != "review_required" {
		t.Fatalf("expected review_required collection, got %v", got)
	}

	reviewQueue := sendJSON(t, mux, http.MethodGet, "/v1/collections?review_only=true", authHdr, nil, http.StatusOK)
	if int(reviewQueue["count"].(float64)) < 1 {
		t.Fatalf("expected review queue item, got %#v", reviewQueue)
	}
	reviewID := mustString(t, reviewResp, "id")
	reviewed := sendJSON(t, mux, http.MethodPost, "/v1/collections/"+reviewID+"/review", authHdr, map[string]any{
		"order_id": orderID,
		"notes":    "matched manually after remittance review",
	}, http.StatusOK)
	if got := reviewed["status"]; got != "matched" {
		t.Fatalf("expected reviewed collection to be matched, got %v", got)
	}
}

func TestIntegrationConnectedAccountSplitsAppearInSettlement(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Marketplace Merchant", Email: "marketplace@test.com", BusinessType: "company",
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

	connectedResp := sendJSON(t, mux, http.MethodPost, "/v1/connected-accounts", authHdr, map[string]any{
		"display_name":       "Seller A",
		"external_reference": "seller-a",
	}, http.StatusCreated)
	connectedID := mustString(t, connectedResp, "id")

	orderResp := sendJSON(t, mux, http.MethodPost, "/v1/orders", authHdr, map[string]any{
		"amount": 10000, "currency": "INR", "receipt": "marketplace-order",
	}, http.StatusCreated)
	tokenID := createCardTokenViaMux(t, mux, authHdr, false)
	paymentResp := sendJSON(t, mux, http.MethodPost, "/v1/payments/authorize", authHdr, map[string]any{
		"order_id":                orderResp["id"],
		"amount":                  10000,
		"currency":                "INR",
		"method":                  "card",
		"payment_method_token_id": tokenID,
		"split_instructions": []map[string]any{
			{"connected_account_id": connectedID, "beneficiary_label": "Seller A", "amount": 4000},
		},
	}, http.StatusCreated)
	paymentID := mustString(t, paymentResp, "id")

	captureResp := sendJSON(t, mux, http.MethodPost, "/v1/payments/"+paymentID+"/capture", authHdr, map[string]any{"amount": 10000}, http.StatusOK)
	splits := captureResp["splits"].([]any)
	if len(splits) != 1 {
		t.Fatalf("expected 1 split on payment response, got %#v", captureResp)
	}

	settlementResp := sendJSON(t, mux, http.MethodPost, "/v1/settlements/partial", authHdr, map[string]any{
		"payment_ids": []string{paymentID},
	}, http.StatusCreated)
	settlementID := mustString(t, settlementResp, "id")
	settlementGet := sendJSON(t, mux, http.MethodGet, "/v1/settlements/"+settlementID, authHdr, nil, http.StatusOK)
	items := settlementGet["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 settlement item, got %#v", settlementGet)
	}
	item := items[0].(map[string]any)
	summary := item["split_summary"].([]any)
	if len(summary) != 1 {
		t.Fatalf("expected split summary on settlement item, got %#v", item)
	}
	split := summary[0].(map[string]any)
	if got := split["beneficiary_label"]; got != "Seller A" {
		t.Fatalf("expected Seller A beneficiary label, got %v", got)
	}
}
