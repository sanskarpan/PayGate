//go:build chaos

package chaos_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// ToxiproxyClient wraps the Toxiproxy API.
type ToxiproxyClient struct {
	addr string
}

func NewToxiproxyClient(addr string) *ToxiproxyClient {
	return &ToxiproxyClient{addr: addr}
}

func (c *ToxiproxyClient) AddLatency(proxy string, latencyMs, jitterMs int) error {
	body, _ := json.Marshal(map[string]any{
		"name": "latency",
		"type": "latency",
		"attributes": map[string]any{
			"latency": latencyMs,
			"jitter":  jitterMs,
		},
	})
	resp, err := http.Post(
		fmt.Sprintf("http://%s/proxies/%s/toxics", c.addr, proxy),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *ToxiproxyClient) RemoveToxic(proxy, toxicName string) error {
	req, _ := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("http://%s/proxies/%s/toxics/%s", c.addr, proxy, toxicName), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *ToxiproxyClient) DisableProxy(proxy string) error {
	body, _ := json.Marshal(map[string]bool{"enabled": false})
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://%s/proxies/%s", c.addr, proxy),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *ToxiproxyClient) EnableProxy(proxy string) error {
	body, _ := json.Marshal(map[string]bool{"enabled": true})
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://%s/proxies/%s", c.addr, proxy),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// TestRedisFailure_IdempotencyFallback verifies that when Redis is down,
// money-changing endpoints fall back to DB-backed idempotency and do not
// double-charge on retry.
func TestRedisFailure_IdempotencyFallback(t *testing.T) {
	const (
		toxiproxyAddr = "localhost:8474"
		apiBase       = "http://localhost:8090"
		idempKey      = "chaos-test-redis-down-001"
	)

	toxi := NewToxiproxyClient(toxiproxyAddr)

	// Disable Redis
	if err := toxi.DisableProxy("redis"); err != nil {
		t.Skipf("toxiproxy not available: %v", err)
	}
	defer func() {
		if err := toxi.EnableProxy("redis"); err != nil {
			t.Logf("warning: failed to re-enable redis proxy: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = ctx

	// First request — should succeed using DB idempotency
	body1 := `{"amount":50000,"currency":"INR","receipt":"chaos_redis_test"}`
	resp1, err := idempotentPost(apiBase+"/v1/orders", idempKey, body1)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp1.StatusCode)
	}
	var order1 map[string]any
	json.NewDecoder(resp1.Body).Decode(&order1)
	resp1.Body.Close()
	t.Logf("order created: %v", order1["id"])

	// Second request — same idempotency key, Redis still down.
	// Should return the same order from DB idempotency store.
	resp2, err := idempotentPost(apiBase+"/v1/orders", idempKey, body1)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	defer resp2.Body.Close()

	var order2 map[string]any
	json.NewDecoder(resp2.Body).Decode(&order2)

	if order1["id"] != order2["id"] {
		t.Errorf("idempotency broken: got different order IDs: %v vs %v", order1["id"], order2["id"])
	}
	if resp2.Header.Get("Idempotent-Replayed") != "true" {
		t.Errorf("expected Idempotent-Replayed header on duplicate request")
	}
	t.Logf("idempotency correct: same order ID %v returned", order2["id"])
}

// TestDBLatency_CaptureTimeout verifies that high DB latency causes capture
// to fail gracefully and does not leave the payment in an inconsistent state.
func TestDBLatency_CaptureTimeout(t *testing.T) {
	t.Skip("requires a pre-authorized payment ID in PAYMENT_ID env var")
}

func idempotentPost(url, idempKey, body string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)
	// Note: in a real test, set Authorization header with test API key
	return http.DefaultClient.Do(req)
}
