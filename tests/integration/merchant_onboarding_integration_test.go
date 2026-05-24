//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/sanskarpan/PayGate/internal/merchant"
)

func TestIntegrationMerchantOnboardingLifecycleAndLiveKeyGate(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name:         "KYB Merchant",
		Email:        uniqueTestEmail(t, "kyb"),
		BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	if createdMerchant.OnboardingStatus != merchant.OnboardingStateDraft {
		t.Fatalf("expected draft onboarding status, got %s", createdMerchant.OnboardingStatus)
	}

	_, err = merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode:  merchant.APIKeyModeLive,
		Scope: merchant.APIKeyScopeAdmin,
	})
	if err == nil {
		t.Fatal("expected live key creation to fail before onboarding approval")
	}

	adminKey, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode:  merchant.APIKeyModeTest,
		Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create admin api key: %v", err)
	}
	authHeader := basicAuth(adminKey.KeyID, adminKey.KeySecret)

	app := sendJSON(t, mux, http.MethodGet, "/v1/merchants/me/onboarding", authHeader, nil, http.StatusOK)
	if got := mustString(t, app, "state"); got != string(merchant.OnboardingStateDraft) {
		t.Fatalf("expected draft state, got %s", got)
	}

	app = sendJSON(t, mux, http.MethodPut, "/v1/merchants/me/onboarding", authHeader, map[string]any{
		"legal_name":              "KYB Merchant Private Limited",
		"business_classification": "private_limited",
		"registration_number":     "U12345KA2026PTC100001",
		"tax_identifier":          "29ABCDE1234F1Z9",
		"country_code":            "IN",
	}, http.StatusOK)
	if got := mustString(t, app, "legal_name"); got != "KYB Merchant Private Limited" {
		t.Fatalf("expected legal name to persist, got %s", got)
	}
	approveMerchantOnboarding(t, mux, authHeader)
	app = sendJSON(t, mux, http.MethodGet, "/v1/merchants/me/onboarding", authHeader, nil, http.StatusOK)
	if got := mustString(t, app, "state"); got != string(merchant.OnboardingStateApproved) {
		t.Fatalf("expected approved state, got %s", got)
	}

	liveKey, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode:  merchant.APIKeyModeLive,
		Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create live key after approval: %v", err)
	}
	if liveKey.KeyID == "" {
		t.Fatal("expected live key id")
	}
}
