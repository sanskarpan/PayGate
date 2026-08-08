package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"syscall"
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

// ErrBlockedWebhookTarget is reported when a webhook endpoint resolves to an
// address the deliverer is not permitted to reach.
var ErrBlockedWebhookTarget = errors.New("webhook target address is blocked")

// dialPolicy decides which resolved IPs a single delivery may connect to. It
// mirrors validateSubscriptionURL: https may only reach public addresses, and
// http may only reach loopback.
type dialPolicy int

const (
	policyPublicOnly dialPolicy = iota
	policyLoopbackOnly
)

type dialPolicyKey struct{}

func withDialPolicy(ctx context.Context, p dialPolicy) context.Context {
	return context.WithValue(ctx, dialPolicyKey{}, p)
}

// dialPolicyFrom fails closed: an unlabelled context gets the strict policy.
func dialPolicyFrom(ctx context.Context) dialPolicy {
	if p, ok := ctx.Value(dialPolicyKey{}).(dialPolicy); ok {
		return p
	}
	return policyPublicOnly
}

// loopbackDeliveryAllowed reports whether http-to-loopback delivery is
// permitted. Local development and the Playwright suite post to
// http://127.0.0.1 receivers, so it stays on outside production.
func loopbackDeliveryAllowed() bool {
	return os.Getenv("APP_ENV") != "production"
}

// checkWebhookDialAddr validates the address the transport is about to connect
// to. It runs after DNS resolution, so a hostname that passed validation at
// subscription time but later resolves to an internal address (DNS rebinding)
// is still refused.
func checkWebhookDialAddr(ctx context.Context, network, address string, allowLoopback bool) error {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return fmt.Errorf("%w: unsupported network %q", ErrBlockedWebhookTarget, network)
	}

	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBlockedWebhookTarget, err)
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("%w: %q is not an IP literal", ErrBlockedWebhookTarget, host)
	}
	addr = addr.Unmap()

	if dialPolicyFrom(ctx) == policyLoopbackOnly {
		if !allowLoopback {
			return fmt.Errorf("%w: http delivery to %s is disabled in production", ErrBlockedWebhookTarget, addr)
		}
		if !addr.IsLoopback() {
			return fmt.Errorf("%w: %s is not loopback (http allows loopback only)", ErrBlockedWebhookTarget, addr)
		}
		return nil
	}

	if isBlockedWebhookAddr(addr) {
		return fmt.Errorf("%w: %s", ErrBlockedWebhookTarget, addr)
	}
	return nil
}

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
//
// The client deliberately refuses redirects and validates every resolved
// address at dial time. Without both, a merchant could register a public
// endpoint that redirects to an internal service (or re-resolve its hostname to
// one) and read the response back through the delivery log.
func NewDeliverer() *Deliverer {
	allowLoopback := loopbackDeliveryAllowed()

	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
		ControlContext: func(ctx context.Context, network, address string, _ syscall.RawConn) error {
			return checkWebhookDialAddr(ctx, network, address, allowLoopback)
		},
	}

	return &Deliverer{
		client: &http.Client{
			Timeout: deliveryTimeout,
			// Proxy is nil on purpose: an env-configured proxy would bypass the dial guard.
			Transport: &http.Transport{
				Proxy:                 nil,
				DialContext:           dialer.DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   4,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: time.Second,
			},
			// Record the 3xx itself as a failed delivery instead of following it.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
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
	switch req.URL.Scheme {
	case "http":
		req = req.WithContext(withDialPolicy(req.Context(), policyLoopbackOnly))
	case "https":
		req = req.WithContext(withDialPolicy(req.Context(), policyPublicOnly))
	default:
		return DeliveryResult{Error: fmt.Sprintf("%v: unsupported scheme %q", ErrBlockedWebhookTarget, req.URL.Scheme)}
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
