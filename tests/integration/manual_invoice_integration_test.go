//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/sanskarpan/PayGate/internal/merchant"
)

func TestIntegrationManualInvoiceSurfaceAndStatusDerivation(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	createdMerchant, err := merchantSvc.CreateMerchant(t.Context(), merchant.CreateMerchantInput{
		Name:         "Manual Invoice Merchant",
		Email:        uniqueTestEmail(t, "manual-invoice"),
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

	customerResp := sendJSON(t, mux, http.MethodPost, "/v1/customers", authHeader, map[string]any{
		"name":  "Invoice Customer",
		"email": "invoice.customer@test.local",
	}, http.StatusCreated)
	customerID := mustString(t, customerResp, "id")

	invoiceResp := sendJSON(t, mux, http.MethodPost, "/v1/invoices", authHeader, map[string]any{
		"customer_id":        customerID,
		"amount":             5100,
		"currency":           "INR",
		"description":        "April consulting invoice",
		"external_reference": "inv-apr-001",
		"collection_surface": "payment_link",
		"due_at":             time.Now().UTC().Add(24 * time.Hour).Unix(),
	}, http.StatusCreated)
	invoiceID := mustString(t, invoiceResp, "id")
	orderID := mustString(t, invoiceResp, "order_id")
	if mustString(t, invoiceResp, "payment_link_id") == "" {
		t.Fatalf("expected payment_link_id on invoice, got %#v", invoiceResp)
	}

	remindedResp := sendJSON(t, mux, http.MethodPost, "/v1/invoices/"+invoiceID+"/send-reminder", authHeader, map[string]any{}, http.StatusOK)
	if got := remindedResp["reminder_count"].(float64); got != 1 {
		t.Fatalf("expected reminder_count 1, got %#v", remindedResp["reminder_count"])
	}

	cardTokenID := createCardTokenViaMux(t, mux, authHeader, false)
	paymentResp := sendJSON(t, mux, http.MethodPost, "/v1/payments/authorize", authHeader, map[string]any{
		"order_id":                orderID,
		"amount":                  5100,
		"currency":                "INR",
		"method":                  "card",
		"payment_method_token_id": cardTokenID,
	}, http.StatusCreated)
	captureResp := sendJSON(t, mux, http.MethodPost, "/v1/payments/"+mustString(t, paymentResp, "id")+"/capture", authHeader, map[string]any{
		"amount": 5100,
	}, http.StatusOK)

	paidInvoice := sendJSON(t, mux, http.MethodGet, "/v1/invoices/"+invoiceID, authHeader, nil, http.StatusOK)
	if got := mustString(t, paidInvoice, "status"); got != "paid" {
		t.Fatalf("expected paid invoice status, got %s", got)
	}
	if got := mustString(t, paidInvoice, "payment_id"); got != mustString(t, captureResp, "id") {
		t.Fatalf("expected derived payment_id %s, got %s", mustString(t, captureResp, "id"), got)
	}

	overdueResp := sendJSON(t, mux, http.MethodPost, "/v1/invoices", authHeader, map[string]any{
		"customer_id":        customerID,
		"amount":             2200,
		"currency":           "INR",
		"description":        "Overdue bank collect invoice",
		"external_reference": "inv-od-001",
		"collection_surface": "virtual_account",
		"due_at":             time.Now().UTC().Add(-24 * time.Hour).Unix(),
	}, http.StatusCreated)
	if mustString(t, overdueResp, "virtual_account_id") == "" {
		t.Fatalf("expected virtual_account_id on overdue invoice, got %#v", overdueResp)
	}
	if overdue, ok := overdueResp["overdue"].(bool); !ok || !overdue {
		t.Fatalf("expected overdue invoice, got %#v", overdueResp["overdue"])
	}
}
