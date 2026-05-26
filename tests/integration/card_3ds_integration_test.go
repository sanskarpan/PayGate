//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
)

func TestIntegrationCard3DSLifecycle(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, orderSvc, _ := buildGatewayMux(db)
	ctx := context.Background()

	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name:         "3DS Merchant",
		Email:        uniqueTestEmail(t, "card-3ds"),
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

	setScenario := func(mode string) {
		sendJSON(t, mux, "POST", "/v1/gateway/scenarios", authHeader, map[string]any{
			"merchant_id": createdMerchant.ID,
			"mode":        mode,
		}, 201)
	}

	createOrderID := func(receipt string, amount int64) string {
		o, err := orderSvc.Create(ctx, order.CreateInput{
			MerchantID: createdMerchant.ID,
			Amount:     amount,
			Currency:   "INR",
			Receipt:    receipt,
		})
		if err != nil {
			t.Fatalf("create order %s: %v", receipt, err)
		}
		return o.ID
	}

	createChallengePayment := func(orderID, tokenID string, amount int64) map[string]any {
		resp := sendJSON(t, mux, "POST", "/v1/payments/authorize", authHeader, map[string]any{
			"order_id":                orderID,
			"amount":                  amount,
			"currency":                "INR",
			"method":                  "card",
			"payment_method_token_id": tokenID,
			"auto_capture":            false,
		}, 201)
		if got := resp["status"]; got != "requires_action" {
			t.Fatalf("expected requires_action, got %#v", got)
		}
		return resp
	}

	t.Run("successful challenge authorizes and can be captured", func(t *testing.T) {
		setScenario("challenge")
		tokenID := createCardTokenViaMux(t, mux, authHeader, false)
		paymentResp := createChallengePayment(createOrderID("card-3ds-success", 5100), tokenID, 5100)
		nextAction, _ := paymentResp["next_action"].(map[string]any)
		if nextAction["type"] != "card_3ds_challenge" {
			t.Fatalf("expected card_3ds_challenge next action, got %#v", nextAction)
		}
		sandbox, _ := paymentResp["sandbox"].(map[string]any)
		callbackURL, _ := sandbox["callback_url"].(string)
		if callbackURL == "" {
			t.Fatalf("expected sandbox callback url, got %#v", sandbox)
		}

		callbackResp := sendJSON(t, mux, "POST", callbackURL, "", map[string]any{
			"merchant_id":       createdMerchant.ID,
			"status":            "succeeded",
			"gateway_reference": "gw_ref_3ds_success",
			"auth_code":         "AUTH_3DS_OK",
		}, 202)
		if processed, _ := callbackResp["processed"].(bool); !processed {
			t.Fatalf("expected callback to process, got %#v", callbackResp)
		}
		paymentMap := callbackResp["payment"].(map[string]any)
		if got := paymentMap["status"]; got != "authorized" {
			t.Fatalf("expected authorized after challenge, got %#v", got)
		}

		duplicateResp := sendJSON(t, mux, "POST", callbackURL, "", map[string]any{
			"merchant_id":       createdMerchant.ID,
			"status":            "succeeded",
			"gateway_reference": "gw_ref_3ds_success",
			"auth_code":         "AUTH_3DS_OK",
		}, 202)
		if processed, _ := duplicateResp["processed"].(bool); processed {
			t.Fatalf("expected duplicate callback to be ignored, got %#v", duplicateResp)
		}

		captured := sendJSON(t, mux, "POST", "/v1/payments/"+mustString(t, paymentResp, "id")+"/capture", authHeader, map[string]any{
			"amount": 5100,
		}, 200)
		if got := captured["status"]; got != "captured" {
			t.Fatalf("expected captured payment, got %#v", got)
		}
	})

	t.Run("abandoned challenge releases single-use token", func(t *testing.T) {
		setScenario("challenge")
		tokenID := createCardTokenViaMux(t, mux, authHeader, false)
		paymentResp := createChallengePayment(createOrderID("card-3ds-abandon", 5200), tokenID, 5200)
		sandbox, _ := paymentResp["sandbox"].(map[string]any)
		callbackURL, _ := sandbox["callback_url"].(string)

		callbackResp := sendJSON(t, mux, "POST", callbackURL, "", map[string]any{
			"merchant_id":       createdMerchant.ID,
			"status":            "abandoned",
			"error_code":        "CARD_3DS_ABANDONED",
			"error_description": "customer closed challenge",
		}, 202)
		if processed, _ := callbackResp["processed"].(bool); !processed {
			t.Fatalf("expected abandoned callback to process, got %#v", callbackResp)
		}
		paymentMap := callbackResp["payment"].(map[string]any)
		if got := paymentMap["status"]; got != "failed" {
			t.Fatalf("expected failed payment after abandonment, got %#v", got)
		}

		setScenario("success")
		reuseResp := sendJSON(t, mux, "POST", "/v1/payments/authorize", authHeader, map[string]any{
			"order_id":                createOrderID("card-3ds-reuse", 5300),
			"amount":                  5300,
			"currency":                "INR",
			"method":                  "card",
			"payment_method_token_id": tokenID,
		}, 201)
		if got := reuseResp["status"]; got != "authorized" {
			t.Fatalf("expected token to be reusable after abandoned challenge, got %#v", reuseResp)
		}
	})

	t.Run("late completion after expiry is ignored safely", func(t *testing.T) {
		setScenario("challenge")
		tokenID := createCardTokenViaMux(t, mux, authHeader, true)
		paymentResp := createChallengePayment(createOrderID("card-3ds-expired", 5400), tokenID, 5400)
		paymentID := mustString(t, paymentResp, "id")
		if _, err := db.Exec(ctx, `
UPDATE paygate_payments.card_challenge_sessions
SET expires_at = $2
WHERE payment_id = $1
`, paymentID, time.Now().UTC().Add(-time.Minute)); err != nil {
			t.Fatalf("expire challenge session: %v", err)
		}
		sandbox, _ := paymentResp["sandbox"].(map[string]any)
		callbackURL, _ := sandbox["callback_url"].(string)

		callbackResp := sendJSON(t, mux, "POST", callbackURL, "", map[string]any{
			"merchant_id":       createdMerchant.ID,
			"status":            "succeeded",
			"gateway_reference": "gw_ref_late_success",
			"auth_code":         "AUTH_LATE",
		}, 202)
		if processed, _ := callbackResp["processed"].(bool); processed {
			t.Fatalf("expected late completion to be ignored, got %#v", callbackResp)
		}
		paymentMap := callbackResp["payment"].(map[string]any)
		if got := paymentMap["status"]; got != "failed" {
			t.Fatalf("expected failed payment after expiry, got %#v", got)
		}
		if got := paymentMap["method_state"]; got != "card_3ds_expired" {
			t.Fatalf("expected card_3ds_expired method state, got %#v", got)
		}
	})
}
