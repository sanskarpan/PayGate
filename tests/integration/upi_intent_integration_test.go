//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
)

func TestIntegrationUPIIntentLifecycle(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, orderSvc, _ := buildGatewayMux(db)
	ctx := context.Background()

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "UPI Merchant", Email: uniqueTestEmail(t, "upi-intent"), BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHeader := basicAuth(key.KeyID, key.KeySecret)

	createOrder := func(receipt string, amount int64) order.Order {
		o, err := orderSvc.Create(ctx, order.CreateInput{
			MerchantID: createdMerchant.ID,
			Amount:     amount,
			Currency:   "INR",
			Receipt:    receipt,
		})
		if err != nil {
			t.Fatalf("create order %s: %v", receipt, err)
		}
		return o
	}

	createIntent := func(orderID string, amount int64, expiresIn int64) map[string]any {
		body, _ := json.Marshal(map[string]any{
			"order_id":           orderID,
			"amount":             amount,
			"currency":           "INR",
			"vpa":                "customer@upi",
			"expires_in_seconds": expiresIn,
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/payments/upi/intents", bytes.NewReader(body))
		req.Header.Set("Authorization", authHeader)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", receiptScopedKey(orderID))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create upi intent: expected 201, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode create upi intent response: %v", err)
		}
		return resp
	}

	t.Run("success and duplicate callback are idempotent", func(t *testing.T) {
		o := createOrder("upi-success", 12345)
		intent := createIntent(o.ID, o.Amount, 120)
		if got := intent["status"]; got != "pending_customer_action" {
			t.Fatalf("expected pending_customer_action, got %v", got)
		}
		nextAction, _ := intent["next_action"].(map[string]any)
		if nextAction["deep_link"] == "" {
			t.Fatalf("expected deep link in next_action, got %#v", nextAction)
		}
		sandbox, _ := intent["sandbox"].(map[string]any)
		callbackURL, _ := sandbox["callback_url"].(string)
		if callbackURL == "" {
			t.Fatalf("expected sandbox callback url, got %#v", sandbox)
		}

		callbackBody, _ := json.Marshal(map[string]any{
			"merchant_id":       createdMerchant.ID,
			"event_id":          "upi_evt_success_once",
			"status":            "succeeded",
			"gateway_reference": "upi_ref_success_once",
		})
		callbackReq := httptest.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(callbackBody))
		callbackReq.Header.Set("Content-Type", "application/json")
		callbackRec := httptest.NewRecorder()
		mux.ServeHTTP(callbackRec, callbackReq)
		if callbackRec.Code != http.StatusAccepted {
			t.Fatalf("upi callback success: expected 202, got %d body=%s", callbackRec.Code, callbackRec.Body.String())
		}

		callbackReq = httptest.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(callbackBody))
		callbackReq.Header.Set("Content-Type", "application/json")
		callbackRec = httptest.NewRecorder()
		mux.ServeHTTP(callbackRec, callbackReq)
		if callbackRec.Code != http.StatusAccepted {
			t.Fatalf("duplicate upi callback: expected 202, got %d body=%s", callbackRec.Code, callbackRec.Body.String())
		}
		var callbackResp map[string]any
		if err := json.Unmarshal(callbackRec.Body.Bytes(), &callbackResp); err != nil {
			t.Fatalf("decode duplicate callback response: %v", err)
		}
		if processed, _ := callbackResp["processed"].(bool); processed {
			t.Fatalf("expected duplicate callback processed=false, got %#v", callbackResp)
		}

		getReq := httptest.NewRequest(http.MethodGet, "/v1/payments/"+intent["id"].(string)+"/upi-intent", nil)
		getReq.Header.Set("Authorization", authHeader)
		getRec := httptest.NewRecorder()
		mux.ServeHTTP(getRec, getReq)
		if getRec.Code != http.StatusOK {
			t.Fatalf("get upi intent: expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
		}
		var current map[string]any
		if err := json.Unmarshal(getRec.Body.Bytes(), &current); err != nil {
			t.Fatalf("decode get upi intent: %v", err)
		}
		if got := current["status"]; got != "captured" {
			t.Fatalf("expected captured after success callback, got %v", got)
		}
		if got := current["provider_status"]; got != "succeeded" {
			t.Fatalf("expected provider_status=succeeded, got %v", got)
		}
	})

	t.Run("timeout poll and late success are deterministic", func(t *testing.T) {
		o := createOrder("upi-timeout", 8800)
		intent := createIntent(o.ID, o.Amount, 1)
		time.Sleep(2 * time.Second)

		pollReq := httptest.NewRequest(http.MethodPost, "/v1/payments/"+intent["id"].(string)+"/upi-intent/poll", nil)
		pollReq.Header.Set("Authorization", authHeader)
		pollRec := httptest.NewRecorder()
		mux.ServeHTTP(pollRec, pollReq)
		if pollRec.Code != http.StatusOK {
			t.Fatalf("poll expired upi intent: expected 200, got %d body=%s", pollRec.Code, pollRec.Body.String())
		}
		var polled map[string]any
		if err := json.Unmarshal(pollRec.Body.Bytes(), &polled); err != nil {
			t.Fatalf("decode poll response: %v", err)
		}
		if got := polled["status"]; got != "failed" {
			t.Fatalf("expected failed status after expiry, got %v", got)
		}
		if got := polled["provider_status"]; got != "expired" {
			t.Fatalf("expected provider_status=expired, got %v", got)
		}

		sandbox, _ := intent["sandbox"].(map[string]any)
		callbackURL, _ := sandbox["callback_url"].(string)
		callbackBody, _ := json.Marshal(map[string]any{
			"merchant_id":       createdMerchant.ID,
			"event_id":          "upi_evt_late_success",
			"status":            "succeeded",
			"gateway_reference": "upi_ref_late_success",
		})
		callbackReq := httptest.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(callbackBody))
		callbackReq.Header.Set("Content-Type", "application/json")
		callbackRec := httptest.NewRecorder()
		mux.ServeHTTP(callbackRec, callbackReq)
		if callbackRec.Code != http.StatusAccepted {
			t.Fatalf("late success callback: expected 202, got %d body=%s", callbackRec.Code, callbackRec.Body.String())
		}
		var callbackResp map[string]any
		if err := json.Unmarshal(callbackRec.Body.Bytes(), &callbackResp); err != nil {
			t.Fatalf("decode late callback response: %v", err)
		}
		if processed, _ := callbackResp["processed"].(bool); processed {
			t.Fatalf("expected late success to be ignored after expiry, got %#v", callbackResp)
		}
	})

	t.Run("customer abandon marks intent terminal", func(t *testing.T) {
		o := createOrder("upi-abandon", 7600)
		intent := createIntent(o.ID, o.Amount, 120)

		abandonBody, _ := json.Marshal(map[string]any{"reason": "customer closed the app"})
		abandonReq := httptest.NewRequest(http.MethodPost, "/v1/payments/"+intent["id"].(string)+"/upi-intent/abandon", bytes.NewReader(abandonBody))
		abandonReq.Header.Set("Authorization", authHeader)
		abandonReq.Header.Set("Content-Type", "application/json")
		abandonRec := httptest.NewRecorder()
		mux.ServeHTTP(abandonRec, abandonReq)
		if abandonRec.Code != http.StatusOK {
			t.Fatalf("abandon upi intent: expected 200, got %d body=%s", abandonRec.Code, abandonRec.Body.String())
		}
		var abandoned map[string]any
		if err := json.Unmarshal(abandonRec.Body.Bytes(), &abandoned); err != nil {
			t.Fatalf("decode abandon response: %v", err)
		}
		if got := abandoned["provider_status"]; got != "abandoned" {
			t.Fatalf("expected provider_status=abandoned, got %v", got)
		}
		if got := abandoned["failure_code"]; got != "CUSTOMER_ABANDONED" {
			t.Fatalf("expected failure code CUSTOMER_ABANDONED, got %v", got)
		}
	})
}

func receiptScopedKey(orderID string) string {
	return "upi-intent-" + orderID
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse callback url: %v", err)
	}
	return parsed
}
