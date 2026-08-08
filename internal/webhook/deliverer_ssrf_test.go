package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// A registered endpoint that 302s to an internal address must not be followed,
// and the internal response body must never reach the merchant's delivery log.
func TestDelivererDoesNotFollowRedirectToInternalAddress(t *testing.T) {
	var internalHits atomic.Int64
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		internalHits.Add(1)
		_, _ = w.Write([]byte(`{"secret":"INTERNAL-METADATA-LEAK"}`))
	}))
	defer internal.Close()

	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/latest/meta-data", http.StatusFound)
	}))
	defer endpoint.Close()

	result := NewDeliverer().Deliver(context.Background(), endpoint.URL, "s", "payment.captured", []byte(`{}`), SignatureModeCompat)

	if result.Succeeded {
		t.Fatal("a 302 must not count as a successful delivery")
	}
	if result.StatusCode != http.StatusFound {
		t.Fatalf("expected the 302 to be recorded, got %d (err=%q)", result.StatusCode, result.Error)
	}
	if got := internalHits.Load(); got != 0 {
		t.Fatalf("redirect target was contacted %d times; it must never be reached", got)
	}
	if strings.Contains(result.ResponseBody, "INTERNAL-METADATA-LEAK") {
		t.Fatalf("redirect target body leaked into the delivery record: %q", result.ResponseBody)
	}
}

// 307/308 preserve method and body, so they are the most dangerous variant.
func TestDelivererDoesNotFollowPreservingRedirect(t *testing.T) {
	var targetHits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits.Add(1)
	}))
	defer target.Close()

	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/x", http.StatusTemporaryRedirect)
	}))
	defer endpoint.Close()

	result := NewDeliverer().Deliver(context.Background(), endpoint.URL, "s", "payment.captured", []byte(`{}`), SignatureModeCompat)
	if result.Succeeded || result.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("expected the 307 to be a failed delivery, got %+v", result)
	}
	if got := targetHits.Load(); got != 0 {
		t.Fatalf("redirect target contacted %d times", got)
	}
}

// End-to-end proof the dial guard is actually wired into the client. A unit test
// of checkWebhookDialAddr alone would still pass if ControlContext were dropped.
func TestDelivererRefusesHTTPSToBlockedAddressEndToEnd(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"secret":"INTERNAL-METADATA-LEAK"}`))
	}))
	defer server.Close()

	// httptest listens on loopback, which is blocked for the https policy.
	result := NewDeliverer().Deliver(context.Background(), server.URL, "s", "payment.captured", []byte(`{}`), SignatureModeCompat)
	if result.Succeeded {
		t.Fatal("https delivery to a blocked address must not succeed")
	}
	if !strings.Contains(result.Error, ErrBlockedWebhookTarget.Error()) {
		t.Fatalf("expected the dial guard to refuse the connection, got err=%q", result.Error)
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("blocked target contacted %d times", got)
	}
}

// DNS rebinding: a name that validated as public at registration time but now
// resolves to an internal address must be refused at dial time.
func TestDialGuardRejectsRebindToInternalAddress(t *testing.T) {
	t.Parallel()

	ctx := withDialPolicy(context.Background(), policyPublicOnly)
	for _, addr := range []string{
		"127.0.0.1:443", "10.0.0.5:443", "192.168.1.10:443", "172.16.0.10:443",
		"169.254.169.254:443", "100.64.1.2:443", "[::1]:443", "[fd00::1]:443",
		"[::ffff:10.0.0.5]:443",
	} {
		if err := checkWebhookDialAddr(ctx, "tcp", addr, true); err == nil {
			t.Fatalf("expected https dial to %s to be refused", addr)
		}
	}
	if err := checkWebhookDialAddr(ctx, "tcp", "93.184.216.34:443", true); err != nil {
		t.Fatalf("expected https dial to a public address to be allowed, got %v", err)
	}
}

func TestDialGuardHTTPAllowsOnlyLoopback(t *testing.T) {
	t.Parallel()

	ctx := withDialPolicy(context.Background(), policyLoopbackOnly)
	for _, addr := range []string{"10.0.0.5:80", "169.254.169.254:80", "192.168.1.10:80"} {
		if err := checkWebhookDialAddr(ctx, "tcp", addr, true); err == nil {
			t.Fatalf("expected http dial to %s to be refused", addr)
		}
	}
	if err := checkWebhookDialAddr(ctx, "tcp4", "127.0.0.1:33001", true); err != nil {
		t.Fatalf("expected http loopback dial to be allowed, got %v", err)
	}
	if err := checkWebhookDialAddr(ctx, "tcp6", "[::1]:33001", true); err != nil {
		t.Fatalf("expected IPv6 loopback dial to be allowed, got %v", err)
	}
	if err := checkWebhookDialAddr(ctx, "tcp", "127.0.0.1:33001", false); err == nil {
		t.Fatal("expected loopback to be refused when loopback delivery is disabled")
	}
}

// Anything unexpected must fail closed rather than open.
func TestDialGuardFailsClosed(t *testing.T) {
	t.Parallel()

	if got := dialPolicyFrom(context.Background()); got != policyPublicOnly {
		t.Fatalf("expected policyPublicOnly for an unlabelled context, got %v", got)
	}
	if err := checkWebhookDialAddr(context.Background(), "tcp", "127.0.0.1:80", true); err == nil {
		t.Fatal("an unlabelled context must use the strict policy")
	}
	ctx := withDialPolicy(context.Background(), policyLoopbackOnly)
	if err := checkWebhookDialAddr(ctx, "unix", "/var/run/docker.sock", true); err == nil {
		t.Fatal("expected a non-tcp network to be refused")
	}
	if err := checkWebhookDialAddr(ctx, "tcp", "localhost:80", true); err == nil {
		t.Fatal("expected a non-literal host to be refused")
	}
}

func TestLoopbackDeliveryDisabledInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	if !loopbackDeliveryAllowed() {
		t.Fatal("loopback delivery must stay enabled outside production")
	}
	t.Setenv("APP_ENV", "production")
	if loopbackDeliveryAllowed() {
		t.Fatal("loopback delivery must be disabled in production")
	}
}

// The local dev stack and the Playwright suite post to http://127.0.0.1
// receivers; that path must keep working through the real client and dialer.
func TestDelivererStillReachesLoopbackOverHTTP(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := NewDeliverer().Deliver(context.Background(), server.URL, "s", "payment.captured", []byte(`{}`), SignatureModeCompat)
	if !result.Succeeded {
		t.Fatalf("loopback http delivery must keep working, got code=%d err=%q", result.StatusCode, result.Error)
	}
}

func TestDelivererRejectsNonHTTPScheme(t *testing.T) {
	t.Parallel()

	result := NewDeliverer().Deliver(context.Background(), "file:///etc/passwd", "s", "payment.captured", []byte(`{}`), SignatureModeCompat)
	if result.Succeeded || result.Error == "" {
		t.Fatalf("expected a non-http scheme to be refused, got %+v", result)
	}
}
