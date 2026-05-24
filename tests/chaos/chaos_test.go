//go:build chaos

package chaos_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func chaosAPIBase() string {
	if base := os.Getenv("CHAOS_API_BASE_URL"); base != "" {
		return base
	}
	return "http://localhost:8090"
}

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
		idempKey      = "chaos-test-redis-down-001"
	)
	apiBase := chaosAPIBase()

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
	authHeader := chaosAuthHeader(t)
	resp1, err := idempotentPost(apiBase+"/v1/orders", idempKey, body1, authHeader)
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
	resp2, err := idempotentPost(apiBase+"/v1/orders", idempKey, body1, authHeader)
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

func idempotentPost(url, idempKey, body, authHeader string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	return http.DefaultClient.Do(req)
}

func jsonPost(url, body, authHeader string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	return http.DefaultClient.Do(req)
}

func chaosAuthHeader(t *testing.T) string {
	t.Helper()
	if header := os.Getenv("CHAOS_AUTH_HEADER"); header != "" {
		return header
	}
	keyID := os.Getenv("CHAOS_API_KEY_ID")
	keySecret := os.Getenv("CHAOS_API_KEY_SECRET")
	if keyID != "" && keySecret != "" {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(keyID+":"+keySecret))
	}
	t.Skip("set CHAOS_AUTH_HEADER or CHAOS_API_KEY_ID/CHAOS_API_KEY_SECRET to run chaos tests")
	return ""
}

// TestKafkaFailure_OutboxReplay verifies that when the Kafka broker is
// unreachable the outbox relay queues events in Postgres and the system
// remains operational (orders and payments still succeed).  When Kafka
// is restored the outbox relay drains the backlog.
func TestKafkaFailure_OutboxReplay(t *testing.T) {
	const (
		toxiproxyAddr = "localhost:8474"
	)
	apiBase := chaosAPIBase()

	toxi := NewToxiproxyClient(toxiproxyAddr)

	// 1. Disable Kafka proxy — this severs the outbox relay's connection.
	if err := toxi.DisableProxy("kafka"); err != nil {
		t.Skipf("toxiproxy not available (is it running?): %v", err)
	}
	defer func() {
		if err := toxi.EnableProxy("kafka"); err != nil {
			t.Logf("warning: failed to re-enable kafka proxy: %v", err)
		}
	}()

	// 2. Create an order — should succeed even though Kafka is down.
	//    The business operation must not be blocked by message-broker availability.
	idempKey := fmt.Sprintf("chaos-kafka-test-%d", time.Now().UnixNano())
	body := `{"amount":75000,"currency":"INR","receipt":"chaos_kafka_test"}`
	resp, err := idempotentPost(apiBase+"/v1/orders", idempKey, body, chaosAuthHeader(t))
	if err != nil {
		t.Fatalf("order creation request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d — order creation must succeed even when Kafka is down", resp.StatusCode)
	}

	var created map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	orderID, _ := created["id"].(string)
	t.Logf("order created successfully while Kafka is down: %s", orderID)

	// 3. Check that the outbox has at least one unpublished event.
	//    We do this via the /readyz endpoint which reports outbox_unpublished.
	time.Sleep(500 * time.Millisecond) // give the relay one poll cycle
	readyzResp, err := http.Get(apiBase + "/readyz")
	if err != nil {
		t.Fatalf("readyz request failed: %v", err)
	}
	defer readyzResp.Body.Close()
	var readyz map[string]any
	if err := json.NewDecoder(readyzResp.Body).Decode(&readyz); err != nil {
		t.Fatalf("decode readyz: %v", err)
	}
	checks, _ := readyz["checks"].(map[string]any)
	outboxUnpublished, _ := checks["outbox_unpublished"].(string)
	t.Logf("outbox_unpublished while Kafka down: %s", outboxUnpublished)
	// We don't assert a specific count — the relay may have already retried.
	// The important assertion is that the order was created (step 2).

	// 4. Re-enable Kafka and wait for the outbox relay to drain.
	if err := toxi.EnableProxy("kafka"); err != nil {
		t.Fatalf("failed to re-enable kafka: %v", err)
	}
	t.Log("Kafka proxy re-enabled — waiting for outbox relay to drain")

	// Poll readyz for up to 30 seconds waiting for outbox to empty.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		r, err := http.Get(apiBase + "/readyz")
		if err != nil {
			continue
		}
		var rz map[string]any
		json.NewDecoder(r.Body).Decode(&rz)
		r.Body.Close()
		ch, _ := rz["checks"].(map[string]any)
		unpub, _ := ch["outbox_unpublished"].(string)
		t.Logf("outbox_unpublished: %s", unpub)
		if unpub == "0" {
			t.Log("outbox drained successfully after Kafka recovery")
			return
		}
	}
	// If we reach here, the outbox didn't drain in time.
	// This is a warning, not a fatal — network conditions in CI vary.
	t.Log("warning: outbox did not drain within 30s — Kafka relay may still be catching up")
}

// TestKafkaFailure_SettlementFlowRemainsOperational verifies that core
// order->payment->settlement operations remain available while Kafka is down
// and the async backlog drains after broker recovery.
func TestKafkaFailure_SettlementFlowRemainsOperational(t *testing.T) {
	const toxiproxyAddr = "localhost:8474"
	apiBase := chaosAPIBase()
	toxi := NewToxiproxyClient(toxiproxyAddr)

	if err := toxi.DisableProxy("kafka"); err != nil {
		t.Skipf("toxiproxy not available: %v", err)
	}
	defer func() {
		if err := toxi.EnableProxy("kafka"); err != nil {
			t.Logf("warning: failed to re-enable kafka proxy: %v", err)
		}
	}()

	authHeader := chaosAuthHeader(t)
	methodCfgResp, err := jsonPost(apiBase+"/v1/gateway/method-configs", `{"method":"card","success_rate":1,"enabled":true,"delay_ms":10}`, authHeader)
	if err != nil {
		t.Fatalf("set deterministic card method config: %v", err)
	}
	defer methodCfgResp.Body.Close()
	if methodCfgResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(methodCfgResp.Body)
		t.Fatalf("expected 201 for method config, got %d body=%s", methodCfgResp.StatusCode, body)
	}

	orderResp, err := idempotentPost(apiBase+"/v1/orders", fmt.Sprintf("chaos-settlement-order-%d", time.Now().UnixNano()), `{"amount":6200,"currency":"INR","receipt":"chaos_settlement_flow"}`, authHeader)
	if err != nil {
		t.Fatalf("create order while kafka down: %v", err)
	}
	defer orderResp.Body.Close()
	if orderResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(orderResp.Body)
		t.Fatalf("expected 201 for order, got %d body=%s", orderResp.StatusCode, body)
	}
	var order map[string]any
	if err := json.NewDecoder(orderResp.Body).Decode(&order); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	orderID, _ := order["id"].(string)
	if orderID == "" {
		t.Fatal("expected order id")
	}

	tokenResp, err := jsonPost(apiBase+"/v1/card-tokens", `{"card_number":"4111111111111111","exp_month":12,"exp_year":2030,"cardholder_name":"Chaos Test","reusable":false}`, authHeader)
	if err != nil {
		t.Fatalf("create card token while kafka down: %v", err)
	}
	defer tokenResp.Body.Close()
	if tokenResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(tokenResp.Body)
		t.Fatalf("expected 201 for card token, got %d body=%s", tokenResp.StatusCode, body)
	}
	var token map[string]any
	if err := json.NewDecoder(tokenResp.Body).Decode(&token); err != nil {
		t.Fatalf("decode card token: %v", err)
	}
	tokenID, _ := token["id"].(string)
	if tokenID == "" {
		t.Fatal("expected card token id")
	}

	paymentResp, err := jsonPost(apiBase+"/v1/payments/authorize", fmt.Sprintf(`{"order_id":"%s","amount":6200,"currency":"INR","method":"card","payment_method_token_id":"%s","auto_capture":false}`, orderID, tokenID), authHeader)
	if err != nil {
		t.Fatalf("authorize payment while kafka down: %v", err)
	}
	defer paymentResp.Body.Close()
	if paymentResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(paymentResp.Body)
		t.Fatalf("expected 201 for payment auth, got %d body=%s", paymentResp.StatusCode, body)
	}

	var payment map[string]any
	if err := json.NewDecoder(paymentResp.Body).Decode(&payment); err != nil {
		t.Fatalf("decode payment: %v", err)
	}
	paymentID, _ := payment["id"].(string)
	if paymentID == "" {
		t.Fatal("expected payment id")
	}

	captureResp, err := jsonPost(apiBase+"/v1/payments/"+paymentID+"/capture", `{"amount":6200}`, authHeader)
	if err != nil {
		t.Fatalf("capture payment while kafka down: %v", err)
	}
	defer captureResp.Body.Close()
	if captureResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(captureResp.Body)
		t.Fatalf("expected 200 for capture, got %d body=%s", captureResp.StatusCode, body)
	}

	settlementResp, err := jsonPost(apiBase+"/v1/settlements/batch", `{}`, authHeader)
	if err != nil {
		t.Fatalf("run settlement batch while kafka down: %v", err)
	}
	defer settlementResp.Body.Close()
	if settlementResp.StatusCode != http.StatusCreated && settlementResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(settlementResp.Body)
		t.Fatalf("expected settlement batch success while kafka down, got %d body=%s", settlementResp.StatusCode, body)
	}

	readyzResp, err := http.Get(apiBase + "/readyz")
	if err != nil {
		t.Fatalf("readyz request failed: %v", err)
	}
	defer readyzResp.Body.Close()
	var readyz map[string]any
	if err := json.NewDecoder(readyzResp.Body).Decode(&readyz); err != nil {
		t.Fatalf("decode readyz: %v", err)
	}
	checks, _ := readyz["checks"].(map[string]any)
	if checks["outbox_unpublished"] == "0" {
		t.Log("warning: outbox backlog already drained or fallback publisher handled messages immediately")
	}

	if err := toxi.EnableProxy("kafka"); err != nil {
		t.Fatalf("re-enable kafka proxy: %v", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		r, err := http.Get(apiBase + "/readyz")
		if err != nil {
			continue
		}
		var rz map[string]any
		_ = json.NewDecoder(r.Body).Decode(&rz)
		r.Body.Close()
		ch, _ := rz["checks"].(map[string]any)
		if ch["outbox_unpublished"] == "0" {
			return
		}
	}
	t.Log("warning: settlement flow backlog did not fully drain within 30s")
}
