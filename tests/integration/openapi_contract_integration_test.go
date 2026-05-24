//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sanskarpan/PayGate/internal/merchant"
	"gopkg.in/yaml.v3"
)

type openAPIDocument struct {
	Paths map[string]map[string]openAPIOperation `yaml:"paths"`
}

type openAPIOperation struct {
	Summary string `yaml:"summary"`
}

func TestIntegrationOpenAPIContractCoreRoutes(t *testing.T) {
	doc := loadOpenAPIDoc(t)
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name:         "OpenAPI Contract Merchant",
		Email:        "openapi-contract@test.com",
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

	createWebhookResp := sendJSON(t, mux, http.MethodPost, "/v1/webhooks", authHeader, map[string]any{
		"url":    "https://example.com/openapi-contract",
		"events": []string{"payment.captured", "dispute.won"},
	}, http.StatusCreated)
	webhookID := mustString(t, createWebhookResp, "id")

	orderResp := sendJSON(t, mux, http.MethodPost, "/v1/orders", authHeader, map[string]any{
		"amount":   15000,
		"currency": "INR",
		"receipt":  "openapi-contract-order",
	}, http.StatusCreated)
	orderID := mustString(t, orderResp, "id")
	cardTokenID := createCardTokenViaMux(t, mux, authHeader, false)

	paymentResp := sendJSON(t, mux, http.MethodPost, "/v1/payments/authorize", authHeader, map[string]any{
		"order_id":                orderID,
		"amount":                  15000,
		"currency":                "INR",
		"method":                  "card",
		"payment_method_token_id": cardTokenID,
	}, http.StatusCreated)
	paymentID := mustString(t, paymentResp, "id")

	sendJSON(t, mux, http.MethodPost, "/v1/payments/"+paymentID+"/capture", authHeader, map[string]any{
		"amount": 15000,
	}, http.StatusOK)

	refundResp := sendJSON(t, mux, http.MethodPost, "/v1/payments/"+paymentID+"/refunds", authHeader, map[string]any{
		"amount": 1200,
		"reason": "requested_by_customer",
	}, http.StatusCreated)
	refundID := mustString(t, refundResp, "id")

	settlementResp := sendJSON(t, mux, http.MethodPost, "/v1/settlements/batch", authHeader, map[string]any{}, http.StatusCreated)
	settlementID := mustString(t, settlementResp, "id")

	approveMerchantOnboarding(t, mux, authHeader)
	beneficiaryID := createApprovedBeneficiary(t, mux, authHeader)
	payoutResp := sendJSON(t, mux, http.MethodPost, "/v1/settlements/"+settlementID+"/payout", authHeader, map[string]any{
		"beneficiary_id": beneficiaryID,
	}, http.StatusCreated)
	payoutID := mustString(t, payoutResp, "id")
	if _, ok := payoutResp["saga_id"]; !ok {
		t.Fatalf("expected payout response to include saga_id, got %#v", payoutResp)
	}

	disputeResp := sendJSON(t, mux, http.MethodPost, "/v1/disputes", authHeader, map[string]any{
		"payment_id":    paymentID,
		"settlement_id": settlementID,
		"reason":        "fraudulent",
		"amount":        15000,
		"currency":      "INR",
	}, http.StatusCreated)
	disputeID := mustString(t, disputeResp, "id")

	testCases := []struct {
		name         string
		method       string
		requestPath  string
		docPath      string
		expectedCode int
		requiredKeys []string
	}{
		{name: "healthz", method: http.MethodGet, requestPath: "/healthz", docPath: "/healthz", expectedCode: http.StatusOK},
		{name: "readyz", method: http.MethodGet, requestPath: "/readyz", docPath: "/readyz", expectedCode: http.StatusOK, requiredKeys: []string{"status", "checks"}},
		{name: "list orders", method: http.MethodGet, requestPath: "/v1/orders", docPath: "/v1/orders", expectedCode: http.StatusOK, requiredKeys: []string{"entity", "count", "items"}},
		{name: "get order", method: http.MethodGet, requestPath: "/v1/orders/" + orderID, docPath: "/v1/orders/{orderID}", expectedCode: http.StatusOK, requiredKeys: []string{"id", "entity", "amount", "currency", "status"}},
		{name: "list payments", method: http.MethodGet, requestPath: "/v1/payments", docPath: "/v1/payments", expectedCode: http.StatusOK, requiredKeys: []string{"entity", "count", "items"}},
		{name: "get payment", method: http.MethodGet, requestPath: "/v1/payments/" + paymentID, docPath: "/v1/payments/{paymentID}", expectedCode: http.StatusOK, requiredKeys: []string{"id", "entity", "order_id", "amount", "currency", "status"}},
		{name: "list payment refunds", method: http.MethodGet, requestPath: "/v1/payments/" + paymentID + "/refunds", docPath: "/v1/payments/{paymentID}/refunds", expectedCode: http.StatusOK, requiredKeys: []string{"entity", "count", "items"}},
		{name: "get refund", method: http.MethodGet, requestPath: "/v1/refunds/" + refundID, docPath: "/v1/refunds/{refundID}", expectedCode: http.StatusOK, requiredKeys: []string{"id", "entity", "payment_id", "amount", "currency", "status"}},
		{name: "list settlements", method: http.MethodGet, requestPath: "/v1/settlements", docPath: "/v1/settlements", expectedCode: http.StatusOK, requiredKeys: []string{"entity", "count", "items"}},
		{name: "get settlement", method: http.MethodGet, requestPath: "/v1/settlements/" + settlementID, docPath: "/v1/settlements/{settlementID}", expectedCode: http.StatusOK, requiredKeys: []string{"id", "entity", "status", "net_amount", "currency", "items"}},
		{name: "get payout", method: http.MethodGet, requestPath: "/v1/payouts/" + payoutID, docPath: "/v1/payouts/{payoutID}", expectedCode: http.StatusOK, requiredKeys: []string{"id", "entity", "settlement_id", "status", "amount", "currency"}},
		{name: "payout events", method: http.MethodGet, requestPath: "/v1/payouts/" + payoutID + "/events", docPath: "/v1/payouts/{payoutID}/events", expectedCode: http.StatusOK, requiredKeys: []string{"entity", "count", "items"}},
		{name: "list webhooks", method: http.MethodGet, requestPath: "/v1/webhooks", docPath: "/v1/webhooks", expectedCode: http.StatusOK, requiredKeys: []string{"entity", "count", "items"}},
		{name: "get webhook", method: http.MethodGet, requestPath: "/v1/webhooks/" + webhookID, docPath: "/v1/webhooks/{webhookID}", expectedCode: http.StatusOK, requiredKeys: []string{"id", "entity", "url", "events", "status"}},
		{name: "webhook deliveries", method: http.MethodGet, requestPath: "/v1/webhooks/" + webhookID + "/deliveries", docPath: "/v1/webhooks/{webhookID}/deliveries", expectedCode: http.StatusOK, requiredKeys: []string{"entity", "count", "items"}},
		{name: "list disputes", method: http.MethodGet, requestPath: "/v1/disputes", docPath: "/v1/disputes", expectedCode: http.StatusOK, requiredKeys: []string{"entity", "count", "items"}},
		{name: "get dispute", method: http.MethodGet, requestPath: "/v1/disputes/" + disputeID, docPath: "/v1/disputes/{disputeID}", expectedCode: http.StatusOK, requiredKeys: []string{"id", "entity", "payment_id", "settlement_id", "status", "reason", "amount", "currency"}},
		{name: "list sagas", method: http.MethodGet, requestPath: "/v1/sagas", docPath: "/v1/sagas", expectedCode: http.StatusOK, requiredKeys: []string{"entity", "count", "items"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assertOpenAPIOperation(t, doc, tc.docPath, tc.method)
			rec := serveRequest(t, mux, tc.method, tc.requestPath, authHeader, nil)
			if rec.Code != tc.expectedCode {
				t.Fatalf("expected %d, got %d body=%s", tc.expectedCode, rec.Code, rec.Body.String())
			}
			if len(tc.requiredKeys) == 0 {
				return
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			for _, key := range tc.requiredKeys {
				if _, ok := body[key]; !ok {
					t.Fatalf("expected response key %q in %s", key, rec.Body.String())
				}
			}
		})
	}
}

func loadOpenAPIDoc(t *testing.T) openAPIDocument {
	t.Helper()
	raw, err := os.ReadFile("../../docs/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi doc: %v", err)
	}
	var doc openAPIDocument
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse openapi doc: %v", err)
	}
	return doc
}

func assertOpenAPIOperation(t *testing.T, doc openAPIDocument, path, method string) {
	t.Helper()
	ops, ok := doc.Paths[path]
	if !ok {
		t.Fatalf("openapi path %s is missing", path)
	}
	if _, ok := ops[strings.ToLower(method)]; !ok {
		t.Fatalf("openapi method %s %s is missing", method, path)
	}
}

func sendJSON(t *testing.T, mux *http.ServeMux, method, requestPath, authHeader string, payload map[string]any, expectedCode int) map[string]any {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
	}
	rec := serveRequest(t, mux, method, requestPath, authHeader, body)
	if rec.Code != expectedCode {
		t.Fatalf("%s %s expected %d, got %d body=%s", method, requestPath, expectedCode, rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		return map[string]any{}
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func serveRequest(t *testing.T, mux *http.ServeMux, method, requestPath, authHeader string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, requestPath, bytes.NewReader(body))
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func mustString(t *testing.T, payload map[string]any, key string) string {
	t.Helper()
	raw, ok := payload[key]
	if !ok {
		t.Fatalf("missing key %q in payload %#v", key, payload)
	}
	value, ok := raw.(string)
	if !ok || strings.TrimSpace(value) == "" {
		t.Fatalf("expected non-empty string for key %q, got %#v", key, raw)
	}
	return value
}
