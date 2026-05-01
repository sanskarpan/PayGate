package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	deliveryTimeout    = 10 * time.Second
	signatureHeader    = "X-PayGate-Signature"
	timestampHeader    = "X-PayGate-Timestamp"
	eventTypeHeader    = "X-PayGate-Event"
)

// DeliveryResult holds the outcome of a single HTTP delivery attempt.
type DeliveryResult struct {
	StatusCode   int
	ResponseBody string
	Error        string
	Succeeded    bool
}

// Deliverer sends signed HTTP POST requests to webhook endpoints.
type Deliverer struct {
	client *http.Client
}

// NewDeliverer creates a Deliverer with a 10-second per-request timeout.
func NewDeliverer() *Deliverer {
	return &Deliverer{
		client: &http.Client{Timeout: deliveryTimeout},
	}
}

// Deliver signs payload with HMAC-SHA256 and POSTs it to url.
// It returns the delivery result regardless of HTTP status code.
// Only network/IO errors set result.Error; a 5xx response is a failed delivery,
// not an error.
func (d *Deliverer) Deliver(ctx context.Context, url, secret, eventType string, payload []byte) DeliveryResult {
	sig := sign(secret, payload)
	ts := fmt.Sprintf("%d", time.Now().Unix())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return DeliveryResult{Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(signatureHeader, "sha256="+sig)
	req.Header.Set(timestampHeader, ts)
	req.Header.Set(eventTypeHeader, eventType)

	resp, err := d.client.Do(req)
	if err != nil {
		return DeliveryResult{Error: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	succeeded := resp.StatusCode >= 200 && resp.StatusCode < 300
	return DeliveryResult{
		StatusCode:   resp.StatusCode,
		ResponseBody: string(body),
		Succeeded:    succeeded,
	}
}

// sign returns the HMAC-SHA256 hex digest of payload using secret as the key.
func sign(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether the given signature matches the expected signature
// for payload signed with secret. Uses constant-time comparison.
func Verify(secret string, payload []byte, signature string) bool {
	expected := "sha256=" + sign(secret, payload)
	return hmac.Equal([]byte(signature), []byte(expected))
}
