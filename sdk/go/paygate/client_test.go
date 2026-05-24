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
