//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/sanskarpan/PayGate/internal/merchant"
	sdk "github.com/sanskarpan/PayGate/sdk/go/paygate"
)

func TestIntegrationSDKClientsAgainstLiveMux(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx := context.Background()
	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "SDK Merchant", Email: uniqueTestEmail(t, "sdk"), BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeWrite,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	client := sdk.New(server.URL, key.KeyID, key.KeySecret)
	order, err := client.CreateOrder(ctx, map[string]any{
		"amount":   4200,
		"currency": "INR",
		"receipt":  "sdk-go-live",
	}, "sdk-go-order")
	if err != nil {
		t.Fatalf("go sdk create order: %v", err)
	}
	if order.ID == "" {
		t.Fatal("expected order id from go sdk")
	}

	cardToken, err := client.CreateCardToken(ctx, map[string]any{
		"card_number":     "4111111111111111",
		"exp_month":       12,
		"exp_year":        2030,
		"cardholder_name": "SDK Go",
		"reusable":        false,
	})
	if err != nil {
		t.Fatalf("go sdk create card token: %v", err)
	}
	if cardToken.ID == "" {
		t.Fatal("expected card token id from go sdk")
	}

	sub, err := client.CreateWebhookSubscription(ctx, map[string]any{
		"url":            server.URL + "/sdk-hook",
		"events":         []string{"payment.captured"},
		"signature_mode": "compat",
	})
	if err != nil {
		t.Fatalf("go sdk create webhook: %v", err)
	}
	if sub.ID == "" {
		t.Fatal("expected webhook subscription id from go sdk")
	}

	auth := base64.StdEncoding.EncodeToString([]byte(key.KeyID + ":" + key.KeySecret))
	js := `
import { createClient } from './sdk/js/paygate.js';
const client = createClient({ baseUrl: process.env.SDK_BASE_URL, keyId: process.env.SDK_KEY_ID, keySecret: process.env.SDK_KEY_SECRET });
const order = await client.createOrder({ amount: 4300, currency: 'INR', receipt: 'sdk-js-live' }, 'sdk-js-order');
if (!order.id) throw new Error('missing order id');
let token;
try {
  token = await client.createCardToken({ card_number: '5555555555554444', exp_month: 12, exp_year: 2031, cardholder_name: 'SDK JS', reusable: false });
} catch (err) {
  throw new Error('createCardToken failed: ' + err.message);
}
if (!token.id) throw new Error('missing card token id');
let webhook;
try {
  webhook = await client.createWebhookSubscription({ url: process.env.SDK_BASE_URL + '/sdk-js-hook', events: ['payment.captured'], signature_mode: 'compat' });
} catch (err) {
  throw new Error('createWebhookSubscription failed: ' + err.message);
}
if (!webhook.id) throw new Error('missing webhook id');
`
	cmd := exec.Command("node", "--input-type=module", "-e", js)
	cmd.Dir = "../.."
	cmd.Env = append(cmd.Environ(),
		"SDK_BASE_URL="+server.URL,
		"SDK_KEY_ID="+key.KeyID,
		"SDK_KEY_SECRET="+key.KeySecret,
		"SDK_AUTH=Basic "+auth,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js sdk live contract test failed: %v\n%s", err, out)
	}
}
