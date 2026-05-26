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
	"github.com/sanskarpan/PayGate/internal/order"
)

func TestIntegrationUPIQRLifecycle(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, orderSvc, _ := buildGatewayMux(db)
	ctx := context.Background()

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "UPI QR Merchant", Email: uniqueTestEmail(t, "upi-qr"), BusinessType: "company",
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

	createQR := func(orderID string, amount int64, qrMode string, expiresIn int64) map[string]any {
		body, _ := json.Marshal(map[string]any{
			"order_id":           orderID,
			"amount":             amount,
			"currency":           "INR",
			"qr_mode":            qrMode,
			"display_name":       "PayGate Test QR",
			"is_reusable":        qrMode == "static",
			"expires_in_seconds": expiresIn,
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/payments/upi/qrs", bytes.NewReader(body))
		req.Header.Set("Authorization", authHeader)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "upi-qr-"+orderID)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create upi qr: expected 201, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode create upi qr response: %v", err)
		}
		return resp
	}

	t.Run("dynamic qr can be created and completed", func(t *testing.T) {
		o := createOrder("upi-qr-success", 1900)
		qr := createQR(o.ID, o.Amount, "dynamic", 180)
		if qr["flow_type"] != "qr" {
			t.Fatalf("expected flow_type=qr, got %#v", qr["flow_type"])
		}
		if qr["qr_mode"] != "dynamic" {
			t.Fatalf("expected qr_mode=dynamic, got %#v", qr["qr_mode"])
		}
		if qr["status"] != "pending_customer_action" {
			t.Fatalf("expected pending_customer_action, got %#v", qr["status"])
		}
		nextAction, _ := qr["next_action"].(map[string]any)
		if nextAction["qr_payload"] == "" {
			t.Fatalf("expected qr payload in next_action, got %#v", nextAction)
		}
		sandbox, _ := qr["sandbox"].(map[string]any)
		callbackURL, _ := sandbox["callback_url"].(string)
		callbackBody, _ := json.Marshal(map[string]any{
			"merchant_id":       createdMerchant.ID,
			"event_id":          "upi_qr_evt_success",
			"status":            "succeeded",
			"gateway_reference": "upi_qr_ref_success",
		})
		callbackReq := httptest.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(callbackBody))
		callbackReq.Header.Set("Content-Type", "application/json")
		callbackRec := httptest.NewRecorder()
		mux.ServeHTTP(callbackRec, callbackReq)
		if callbackRec.Code != http.StatusAccepted {
			t.Fatalf("upi qr callback success: expected 202, got %d body=%s", callbackRec.Code, callbackRec.Body.String())
		}

		getReq := httptest.NewRequest(http.MethodGet, "/v1/payments/"+qr["id"].(string)+"/upi-qr", nil)
		getReq.Header.Set("Authorization", authHeader)
		getRec := httptest.NewRecorder()
		mux.ServeHTTP(getRec, getReq)
		if getRec.Code != http.StatusOK {
			t.Fatalf("get upi qr: expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
		}
		var current map[string]any
		if err := json.Unmarshal(getRec.Body.Bytes(), &current); err != nil {
			t.Fatalf("decode get upi qr: %v", err)
		}
		if got := current["status"]; got != "captured" {
			t.Fatalf("expected captured after success callback, got %v", got)
		}
	})

	t.Run("expired qr does not settle and late success is ignored", func(t *testing.T) {
		o := createOrder("upi-qr-expired", 2100)
		qr := createQR(o.ID, o.Amount, "dynamic", 1)
		time.Sleep(2 * time.Second)

		pollReq := httptest.NewRequest(http.MethodPost, "/v1/payments/"+qr["id"].(string)+"/upi-qr/poll", nil)
		pollReq.Header.Set("Authorization", authHeader)
		pollRec := httptest.NewRecorder()
		mux.ServeHTTP(pollRec, pollReq)
		if pollRec.Code != http.StatusOK {
			t.Fatalf("poll expired upi qr: expected 200, got %d body=%s", pollRec.Code, pollRec.Body.String())
		}
		var polled map[string]any
		if err := json.Unmarshal(pollRec.Body.Bytes(), &polled); err != nil {
			t.Fatalf("decode qr poll response: %v", err)
		}
		if got := polled["provider_status"]; got != "expired" {
			t.Fatalf("expected provider_status=expired, got %v", got)
		}

		sandbox, _ := qr["sandbox"].(map[string]any)
		callbackURL, _ := sandbox["callback_url"].(string)
		callbackBody, _ := json.Marshal(map[string]any{
			"merchant_id":       createdMerchant.ID,
			"event_id":          "upi_qr_evt_late_success",
			"status":            "succeeded",
			"gateway_reference": "upi_qr_ref_late_success",
		})
		callbackReq := httptest.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(callbackBody))
		callbackReq.Header.Set("Content-Type", "application/json")
		callbackRec := httptest.NewRecorder()
		mux.ServeHTTP(callbackRec, callbackReq)
		if callbackRec.Code != http.StatusAccepted {
			t.Fatalf("late qr success callback: expected 202, got %d body=%s", callbackRec.Code, callbackRec.Body.String())
		}
		var callbackResp map[string]any
		if err := json.Unmarshal(callbackRec.Body.Bytes(), &callbackResp); err != nil {
			t.Fatalf("decode late qr callback response: %v", err)
		}
		if processed, _ := callbackResp["processed"].(bool); processed {
			t.Fatalf("expected late qr success to be ignored, got %#v", callbackResp)
		}
	})

	t.Run("static qr exposes reuse metadata", func(t *testing.T) {
		o := createOrder("upi-qr-static", 3300)
		qr := createQR(o.ID, o.Amount, "static", 300)
		if qr["qr_mode"] != "static" {
			t.Fatalf("expected static qr mode, got %#v", qr["qr_mode"])
		}
		if qr["is_reusable"] != true {
			t.Fatalf("expected static qr reusable=true, got %#v", qr["is_reusable"])
		}
		if qr["display_name"] != "PayGate Test QR" {
			t.Fatalf("expected display name, got %#v", qr["display_name"])
		}
	})
}
