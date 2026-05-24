package metrics

import (
	"net/http/httptest"
	"testing"
)

func TestNormalizeHTTPPathUsesPattern(t *testing.T) {
	req := httptest.NewRequest("GET", "http://localhost/v1/payments/pay_123/refunds", nil)
	req.Pattern = "GET /v1/payments/{paymentID}/refunds"
	if got := normalizeHTTPPath(req); got != "GET /v1/payments/{paymentID}/refunds" {
		t.Fatalf("expected request pattern, got %q", got)
	}
}

func TestNormalizeHTTPPathSanitizesDynamicSegments(t *testing.T) {
	req := httptest.NewRequest("GET", "http://localhost/v1/sagas/saga_1234567890abcdef/actions", nil)
	if got := normalizeHTTPPath(req); got != "/v1/sagas/{id}/actions" {
		t.Fatalf("unexpected normalized path: %q", got)
	}
}
