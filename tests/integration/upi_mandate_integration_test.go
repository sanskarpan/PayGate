//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/sanskarpan/PayGate/internal/merchant"
)

func TestIntegrationUPIMandateLifecycleAndRecurringCharge(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Mandate Merchant", Email: uniqueTestEmail(t, "mandate"), BusinessType: "company",
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

	customerResp := sendJSON(t, mux, "POST", "/v1/customers", authHdr, map[string]any{
		"name":  "Mandate Customer",
		"email": "mandate-customer@test.com",
	}, 201)
	customerID := mustString(t, customerResp, "id")

	mandateResp := sendJSON(t, mux, "POST", "/v1/upi-mandates", authHdr, map[string]any{
		"customer_id":        customerID,
		"reference":          "mandate-gold-1",
		"display_name":       "Gold AutoPay",
		"vpa":                "autopay.customer@upi",
		"amount_limit":       25000,
		"currency":           "INR",
		"interval_unit":      "month",
		"interval_count":     1,
		"retry_window_hours": 12,
		"expires_at":         time.Now().UTC().AddDate(1, 0, 0).Unix(),
	}, 201)
	mandateID := mustString(t, mandateResp, "id")
	if got := mandateResp["status"]; got != "pending_approval" {
		t.Fatalf("expected pending_approval, got %v", got)
	}

	activated := sendJSON(t, mux, "POST", "/v1/upi-mandates/"+mandateID+"/activate", authHdr, map[string]any{
		"reason": "customer approved mandate in sandbox",
	}, 200)
	if got := activated["status"]; got != "active" {
		t.Fatalf("expected active mandate, got %v", got)
	}

	eventsResp := sendJSON(t, mux, "GET", "/v1/upi-mandates/"+mandateID+"/events", authHdr, nil, 200)
	if int(eventsResp["count"].(float64)) < 2 {
		t.Fatalf("expected create and activate events, got %#v", eventsResp)
	}

	subscriptionResp := sendJSON(t, mux, "POST", "/v1/subscriptions", authHdr, map[string]any{
		"customer_id":          customerID,
		"plan_name":            "UPI Gold Monthly",
		"collection_method":    "upi_mandate",
		"upi_mandate_id":       mandateID,
		"amount":               14900,
		"currency":             "INR",
		"interval_unit":        "month",
		"interval_count":       1,
		"starts_at":            time.Now().UTC().Add(-time.Minute).Unix(),
		"max_retry_count":      2,
		"retry_interval_hours": 2,
	}, 201)
	subscriptionID := mustString(t, subscriptionResp, "id")
	if got := subscriptionResp["collection_method"]; got != "upi_mandate" {
		t.Fatalf("expected upi_mandate collection method, got %v", got)
	}

	runResp := sendJSON(t, mux, "POST", "/v1/subscriptions/"+subscriptionID+"/run", authHdr, map[string]any{}, 200)
	invoiceMap := runResp["invoice"].(map[string]any)
	if got := invoiceMap["status"]; got != "paid" {
		t.Fatalf("expected paid invoice, got %v", got)
	}
	paymentMap := runResp["payment"].(map[string]any)
	if paymentMap["captured"] != true {
		t.Fatalf("expected captured mandate charge, got %#v", paymentMap)
	}

	eventsResp = sendJSON(t, mux, "GET", "/v1/upi-mandates/"+mandateID+"/events", authHdr, nil, 200)
	foundCharge := false
	for _, raw := range eventsResp["items"].([]any) {
		item := raw.(map[string]any)
		if item["event_type"] == "charge_succeeded" {
			foundCharge = true
			break
		}
	}
	if !foundCharge {
		t.Fatalf("expected charge_succeeded event, got %#v", eventsResp)
	}

	revoked := sendJSON(t, mux, "POST", "/v1/upi-mandates/"+mandateID+"/revoke", authHdr, map[string]any{
		"reason": "customer revoked mandate",
	}, 200)
	if got := revoked["status"]; got != "revoked" {
		t.Fatalf("expected revoked mandate, got %v", got)
	}

	_ = sendJSON(t, mux, "POST", "/v1/subscriptions/"+subscriptionID+"/run", authHdr, map[string]any{}, 400)
}
