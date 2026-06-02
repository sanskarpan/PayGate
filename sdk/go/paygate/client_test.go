package paygate

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateOrderSetsAuthAndIdempotency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Fatal("missing authorization header")
		}
		if got := r.Header.Get("Idempotency-Key"); got != "idem-1" {
			t.Fatalf("unexpected idempotency key %q", got)
		}
		_ = json.NewEncoder(w).Encode(Order{ID: "order_123", Amount: 4200, Currency: "INR", Receipt: "r1", Status: "created"})
	}))
	defer server.Close()

	client := New(server.URL, "key", "secret")
	order, err := client.CreateOrder(context.Background(), map[string]any{
		"amount":   4200,
		"currency": "INR",
		"receipt":  "r1",
	}, "idem-1")
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.ID == "" {
		t.Fatal("expected order id")
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	body := []byte(`{"id":"evt_1"}`)
	if VerifyWebhookSignature("secret", body, "bad") {
		t.Fatal("expected invalid signature to fail")
	}
	expected := "16abb10adb33ff9cff34f6a57fc2c0b902c11ea19fe73dae86f2940c235e7ed5"
	if !VerifyWebhookSignature("secret", body, expected) {
		t.Fatal("expected valid signature to verify")
	}
	if got := hex.EncodeToString(body); got == "" {
		t.Fatal("expected body hex for sanity check")
	}
}

func TestCreateCardTokenAndWebhookSubscription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/card-tokens":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(CardToken{ID: "ctok_123", Brand: "visa", Last4: "1111", PaymentTokenType: "single_use", Reusable: false})
			case "/v1/webhooks":
				if r.Method != http.MethodPost {
					t.Fatalf("unexpected method %s", r.Method)
				}
				//nolint:gosec // test fixture intentionally mirrors the webhook response shape.
				_ = json.NewEncoder(w).Encode(WebhookSubscription{ID: "wh_123", URL: "https://example.com/hook", Status: "active", SignatureMode: "compat"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(server.URL, "key", "secret")
	cardToken, err := client.CreateCardToken(context.Background(), map[string]any{
		"card_number": "4111111111111111",
		"exp_month":   12,
		"exp_year":    2030,
	})
	if err != nil {
		t.Fatalf("create card token: %v", err)
	}
	if cardToken.ID == "" {
		t.Fatal("expected card token id")
	}

	sub, err := client.CreateWebhookSubscription(context.Background(), map[string]any{
		"url":            "https://example.com/hook",
		"events":         []string{"payment.captured"},
		"signature_mode": "compat",
	})
	if err != nil {
		t.Fatalf("create webhook subscription: %v", err)
	}
	if sub.ID == "" {
		t.Fatal("expected webhook subscription id")
	}
}
