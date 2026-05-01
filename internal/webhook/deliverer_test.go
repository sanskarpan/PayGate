package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSignAndVerify(t *testing.T) {
	secret := "test-secret-abc123"
	payload := []byte(`{"event_type":"payment.captured","payment_id":"pay_abc"}`)

	sig := "sha256=" + sign(secret, payload)

	// Correct signature verifies.
	if !Verify(secret, payload, sig) {
		t.Fatal("expected signature to verify")
	}

	// Tampered payload fails.
	if Verify(secret, []byte("tampered"), sig) {
		t.Fatal("expected tampered payload to fail verification")
	}

	// Wrong secret fails.
	if Verify("wrong-secret", payload, sig) {
		t.Fatal("expected wrong secret to fail verification")
	}

	// Empty signature fails.
	if Verify(secret, payload, "") {
		t.Fatal("expected empty signature to fail verification")
	}
}

func TestDelivererSuccessfulDelivery(t *testing.T) {
	received := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content type")
		}
		sig := r.Header.Get(signatureHeader)
		if sig == "" {
			t.Error("expected X-PayGate-Signature header")
		}
		payload := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(payload)
		received <- payload
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"received":true}`))
	}))
	defer server.Close()

	d := NewDeliverer()
	payload, _ := json.Marshal(map[string]any{"event_type": "payment.captured"})
	result := d.Deliver(t.Context(), server.URL, "my-secret", "payment.captured", payload)

	if !result.Succeeded {
		t.Fatalf("expected successful delivery, got error=%q code=%d", result.Error, result.StatusCode)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", result.StatusCode)
	}
}

func TestDelivererFailsOn4xx5xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	d := NewDeliverer()
	result := d.Deliver(t.Context(), server.URL, "secret", "payment.captured", []byte(`{}`))

	if result.Succeeded {
		t.Fatal("expected failed delivery on 500")
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", result.StatusCode)
	}
}

func TestDelivererNetworkError(t *testing.T) {
	d := NewDeliverer()
	// Use a port that is not listening.
	result := d.Deliver(t.Context(), "http://127.0.0.1:19999/webhook", "secret", "payment.captured", []byte(`{}`))

	if result.Succeeded {
		t.Fatal("expected network error to fail delivery")
	}
	if result.Error == "" {
		t.Error("expected non-empty error message on network failure")
	}
}
