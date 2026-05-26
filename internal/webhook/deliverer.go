package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	deliveryTimeout      = 10 * time.Second
	signatureHeader      = "X-PayGate-Signature"
	timestampHeader      = "X-PayGate-Timestamp"
	eventTypeHeader      = "X-PayGate-Event"
	contentDigestHeader  = "Content-Digest"
	signatureInputHeader = "Signature-Input"
	httpSignatureHeader  = "Signature"
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
func (d *Deliverer) Deliver(ctx context.Context, url, secret, eventType string, payload []byte, mode SignatureMode) DeliveryResult {
	sig := sign(secret, payload)
	createdAt := time.Now().Unix()
	ts := fmt.Sprintf("%d", createdAt)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return DeliveryResult{Error: err.Error()}
	}
	digest := contentDigest(payload)
	signatureInput := structuredSignatureInput(createdAt)
	httpSig := structuredSignature(secret, req.Method, canonicalPath(req), digest, ts, eventType, signatureInput)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(timestampHeader, ts)
	req.Header.Set(eventTypeHeader, eventType)
	switch mode {
	case "", SignatureModeCompat:
		req.Header.Set(signatureHeader, "sha256="+sig)
		req.Header.Set(contentDigestHeader, digest)
		req.Header.Set(signatureInputHeader, signatureInput)
		req.Header.Set(httpSignatureHeader, httpSig)
	case SignatureModeHMAC:
		req.Header.Set(signatureHeader, "sha256="+sig)
	case SignatureModeHTTPMessage:
		req.Header.Set(contentDigestHeader, digest)
		req.Header.Set(signatureInputHeader, signatureInput)
		req.Header.Set(httpSignatureHeader, httpSig)
	default:
		req.Header.Set(signatureHeader, "sha256="+sig)
		req.Header.Set(contentDigestHeader, digest)
		req.Header.Set(signatureInputHeader, signatureInput)
		req.Header.Set(httpSignatureHeader, httpSig)
	}

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

func VerifyHTTPMessageSignature(secret string, req *http.Request, payload []byte) bool {
	signatureInput := req.Header.Get(signatureInputHeader)
	signature := req.Header.Get(httpSignatureHeader)
	if signatureInput == "" || signature == "" {
		return false
	}
	digest := req.Header.Get(contentDigestHeader)
	ts := req.Header.Get(timestampHeader)
	eventType := req.Header.Get(eventTypeHeader)
	if digest == "" || digest != contentDigest(payload) {
		return false
	}
	expected := structuredSignature(secret, req.Method, canonicalPath(req), digest, ts, eventType, signatureInput)
	return hmac.Equal([]byte(signature), []byte(expected))
}

func contentDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha-256=:" + base64.StdEncoding.EncodeToString(sum[:]) + ":"
}

func structuredSignatureInput(createdAt int64) string {
	return fmt.Sprintf(`paygate=("@method" "@path" "content-digest" "x-paygate-timestamp" "x-paygate-event");created=%d;alg="hmac-sha256";keyid="webhook"`, createdAt)
}

func structuredSignature(secret, method, path, digest, ts, eventType, signatureInput string) string {
	base := fmt.Sprintf(
		"@method: %s\n@path: %s\ncontent-digest: %s\nx-paygate-timestamp: %s\nx-paygate-event: %s\n@signature-params: %s",
		method,
		path,
		digest,
		ts,
		eventType,
		signatureInput,
	)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(base))
	return "paygate=:" + base64.StdEncoding.EncodeToString(mac.Sum(nil)) + ":"
}

func canonicalPath(req *http.Request) string {
	path := req.URL.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}
