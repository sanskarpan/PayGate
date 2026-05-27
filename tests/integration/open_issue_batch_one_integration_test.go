//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/payment"
)

func TestIntegrationSavedCardsMetadataAndCustomerDefault(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Saved Card Merchant", Email: uniqueTestEmail(t, "saved-card"), BusinessType: "company",
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
	authHeader := basicAuth(key.KeyID, key.KeySecret)

	customer := sendJSON(t, mux, http.MethodPost, "/v1/customers", authHeader, map[string]any{
		"name": "Card Owner",
	}, http.StatusCreated)
	customerID := mustString(t, customer, "id")

	expires := time.Now().UTC().AddDate(3, 0, 0)
	token := sendJSON(t, mux, http.MethodPost, "/v1/card-tokens", authHeader, map[string]any{
		"card_number":       "5555555555554444",
		"exp_month":         int(expires.Month()),
		"exp_year":          expires.Year(),
		"reusable":          true,
		"customer_ref":      customerID,
		"network_reference": "ntok_live_12345",
	}, http.StatusCreated)
	tokenID := mustString(t, token, "id")
	if got := token["issuer_name"]; got == "" {
		t.Fatalf("expected issuer_name in token response, got %#v", token)
	}
	if got := token["network_token_type"]; got != "network_token" {
		t.Fatalf("expected network_token_type=network_token, got %v", got)
	}

	list := sendJSON(t, mux, http.MethodGet, "/v1/card-tokens?customer_ref="+customerID, authHeader, nil, http.StatusOK)
	if int(list["count"].(float64)) != 1 {
		t.Fatalf("expected one saved card, got %#v", list)
	}

	updatedCustomer := sendJSON(t, mux, http.MethodPatch, "/v1/customers/"+customerID, authHeader, map[string]any{
		"name":                     "Card Owner",
		"default_payment_token_id": tokenID,
	}, http.StatusOK)
	if got := updatedCustomer["default_payment_token_id"]; got != tokenID {
		t.Fatalf("expected default token id %s, got %v", tokenID, got)
	}

	orderOut, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: m.ID, Amount: 5400, Currency: "INR", Receipt: "saved-card-pay",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	authResult, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID:           m.ID,
		OrderID:              orderOut.ID,
		Amount:               orderOut.Amount,
		Currency:             orderOut.Currency,
		Method:               "card",
		PaymentMethodTokenID: tokenID,
	})
	if err != nil {
		t.Fatalf("authorize card: %v", err)
	}
	paymentResp := sendJSON(t, mux, http.MethodGet, "/v1/payments/"+authResult.PaymentID, authHeader, nil, http.StatusOK)
	card := paymentResp["card"].(map[string]any)
	if got := card["issuer_country"]; got != "IN" {
		t.Fatalf("expected issuer_country=IN, got %v", got)
	}
	if got := card["funding_type"]; got == "" {
		t.Fatalf("expected funding_type in payment response, got %#v", card)
	}

	disabled := sendJSON(t, mux, http.MethodDelete, "/v1/card-tokens/"+tokenID, authHeader, map[string]any{
		"reason": "customer removed card",
	}, http.StatusOK)
	if got := disabled["status"]; got != "disabled" {
		t.Fatalf("expected disabled token, got %#v", disabled)
	}
}

func TestIntegrationBeneficiaryPennyDropAndPayoutBatchReadback(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	mux, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	_, authHeader, settlementOut, _ := createSettledMerchantFlow(t, ctx, db, merchantSvc, orderSvc, paymentSvc)

	beneficiary := sendJSON(t, mux, http.MethodPost, "/v1/beneficiaries", authHeader, map[string]any{
		"destination_type":    "bank_account",
		"account_holder_name": "Batch Owner",
		"bank_account_number": "123456789012",
		"bank_ifsc":           "HDFC0001234",
	}, http.StatusCreated)
	beneficiaryID := mustString(t, beneficiary, "id")

	verify := sendJSON(t, mux, http.MethodPost, "/v1/beneficiaries/"+beneficiaryID+"/verify", authHeader, map[string]any{}, http.StatusOK)
	verification := verify["verification"].(map[string]any)
	if got := verification["provider"]; got != "penny_drop" {
		t.Fatalf("expected penny_drop provider, got %v", got)
	}
	evidence := verification["evidence"].(map[string]any)
	if got := evidence["method"]; got != "penny_drop" {
		t.Fatalf("expected penny_drop evidence method, got %v", got)
	}

	sendJSON(t, mux, http.MethodPost, "/v1/beneficiaries/"+beneficiaryID+"/approve", authHeader, map[string]any{
		"notes": "approved for batch payouts",
	}, http.StatusOK)

	batch := sendJSON(t, mux, http.MethodPost, "/v1/payout-batches", authHeader, map[string]any{
		"dry_run": true,
		"items": []map[string]any{
			{
				"settlement_id":  settlementOut.ID,
				"beneficiary_id": beneficiaryID,
			},
		},
	}, http.StatusCreated)
	batchID := mustString(t, batch, "id")

	batches := sendJSON(t, mux, http.MethodGet, "/v1/payout-batches", authHeader, nil, http.StatusOK)
	if int(batches["count"].(float64)) < 1 {
		t.Fatalf("expected payout batch list to include created batch, got %#v", batches)
	}

	batchGet := sendJSON(t, mux, http.MethodGet, "/v1/payout-batches/"+batchID, authHeader, nil, http.StatusOK)
	if got := batchGet["id"]; got != batchID {
		t.Fatalf("expected batch id %s, got %v", batchID, got)
	}
	items := batchGet["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one batch item, got %#v", batchGet)
	}
}
