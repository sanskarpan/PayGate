//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
)

func TestIntegrationUPIVPAValidationAndVerification(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, orderSvc, _ := buildGatewayMux(db)
	ctx := context.Background()

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name:         "UPI Verification Merchant",
		Email:        uniqueTestEmail(t, "upi-verification"),
		BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode:  merchant.APIKeyModeTest,
		Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHeader := basicAuth(key.KeyID, key.KeySecret)

	t.Run("invalid vpa is rejected for intent and mandate creation", func(t *testing.T) {
		orderOut, err := orderSvc.Create(ctx, order.CreateInput{
			MerchantID: createdMerchant.ID,
			Amount:     1800,
			Currency:   "INR",
			Receipt:    "upi-vpa-invalid",
		})
		if err != nil {
			t.Fatalf("create order: %v", err)
		}
		sendJSON(t, mux, "POST", "/v1/payments/upi/intents", authHeader, map[string]any{
			"order_id": orderOut.ID,
			"amount":   1800,
			"currency": "INR",
			"vpa":      "bad vpa",
		}, 400)

		customerResp := sendJSON(t, mux, "POST", "/v1/customers", authHeader, map[string]any{
			"name":  "Invalid Mandate Customer",
			"email": uniqueTestEmail(t, "invalid-mandate"),
		}, 201)
		sendJSON(t, mux, "POST", "/v1/upi-mandates", authHeader, map[string]any{
			"customer_id":        mustString(t, customerResp, "id"),
			"reference":          "invalid-mandate-vpa",
			"display_name":       "Invalid Mandate",
			"vpa":                "broken-vpa",
			"amount_limit":       20000,
			"currency":           "INR",
			"interval_unit":      "month",
			"interval_count":     1,
			"retry_window_hours": 24,
			"expires_at":         time.Now().UTC().AddDate(1, 0, 0).Unix(),
		}, 400)
	})

	t.Run("mandate activation uses fresh verification and exposes versioned record", func(t *testing.T) {
		customerResp := sendJSON(t, mux, "POST", "/v1/customers", authHeader, map[string]any{
			"name":  "Mandate Verification Customer",
			"email": uniqueTestEmail(t, "valid-mandate"),
		}, 201)
		mandateResp := sendJSON(t, mux, "POST", "/v1/upi-mandates", authHeader, map[string]any{
			"customer_id":        mustString(t, customerResp, "id"),
			"reference":          "valid-mandate-vpa",
			"display_name":       "Verified Mandate",
			"vpa":                "merchant.customer@upi",
			"amount_limit":       20000,
			"currency":           "INR",
			"interval_unit":      "month",
			"interval_count":     1,
			"retry_window_hours": 24,
			"expires_at":         time.Now().UTC().AddDate(1, 0, 0).Unix(),
		}, 201)
		latest, ok := mandateResp["latest_verification"].(map[string]any)
		if !ok || latest["id"] == "" {
			t.Fatalf("expected latest verification on mandate create, got %#v", mandateResp)
		}
		if got := latest["version"]; got != float64(1) {
			t.Fatalf("expected verification version 1, got %#v", got)
		}

		activated := sendJSON(t, mux, "POST", "/v1/upi-mandates/"+mustString(t, mandateResp, "id")+"/activate", authHeader, map[string]any{
			"reason": "customer approved",
		}, 200)
		activatedVerification, ok := activated["latest_verification"].(map[string]any)
		if !ok || activatedVerification["id"] == "" {
			t.Fatalf("expected verification on mandate activation, got %#v", activated)
		}
		if got := activatedVerification["status"]; got != "verified" {
			t.Fatalf("expected verified status, got %#v", got)
		}
	})

	t.Run("vpa beneficiary verification carries shared vpa verification evidence", func(t *testing.T) {
		beneficiaryResp := sendJSON(t, mux, "POST", "/v1/beneficiaries", authHeader, map[string]any{
			"destination_type":    "vpa",
			"account_holder_name": "Payout Beneficiary",
			"vpa":                 "beneficiary.payout@upi",
		}, 201)
		result := sendJSON(t, mux, "POST", "/v1/beneficiaries/"+mustString(t, beneficiaryResp, "id")+"/verify", authHeader, map[string]any{}, 200)
		verification, ok := result["verification"].(map[string]any)
		if !ok {
			t.Fatalf("expected verification payload, got %#v", result)
		}
		evidence, _ := verification["evidence"].(map[string]any)
		if evidence["vpa_verification_id"] == "" {
			t.Fatalf("expected vpa verification id in evidence, got %#v", evidence)
		}
	})
}
