package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sanskarpan/PayGate/internal/merchant"
)

type fakeVerifier struct {
	ret merchant.APIKey
	err error
}

func (f fakeVerifier) AuthenticateAPIKey(_ context.Context, _, _ string, _ merchant.APIKeyScope) (merchant.APIKey, error) {
	if f.err != nil {
		return merchant.APIKey{}, f.err
	}
	return f.ret, nil
}

func (f fakeVerifier) AuthenticateDashboardSession(_ context.Context, _ string, _ merchant.APIKeyScope) (merchant.MerchantUser, error) {
	if f.err != nil {
		return merchant.MerchantUser{}, f.err
	}
	return merchant.MerchantUser{}, merchant.ErrDashboardSession
}

func TestRequireScopeUnauthorized(t *testing.T) {
	m := NewMiddleware(fakeVerifier{})
	h := m.RequireScope(merchant.APIKeyScopeRead, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestRequireScopeSuccess(t *testing.T) {
	m := NewMiddleware(fakeVerifier{ret: merchant.APIKey{ID: "rzp_test_x", MerchantID: "merch_x", Scope: merchant.APIKeyScopeRead}})
	h := m.RequireScope(merchant.APIKeyScopeRead, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	token := base64.StdEncoding.EncodeToString([]byte("rzp_test_x:secret"))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic "+token)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRequireScopeRejectsSpoofedForwardedForWhenProxyUntrusted(t *testing.T) {
	m := NewMiddleware(fakeVerifier{ret: merchant.APIKey{
		ID:         "rzp_test_x",
		MerchantID: "merch_x",
		Scope:      merchant.APIKeyScopeRead,
		AllowedIPs: []string{"203.0.113.10"},
	}})
	h := m.RequireScope(merchant.APIKeyScopeRead, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	token := base64.StdEncoding.EncodeToString([]byte("rzp_test_x:secret"))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic "+token)
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.RemoteAddr = "198.51.100.7:4321"

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestRequireScopeUsesForwardedForFromTrustedProxy(t *testing.T) {
	m := NewMiddlewareWithTrustedProxyCIDRs(fakeVerifier{ret: merchant.APIKey{
		ID:         "rzp_test_x",
		MerchantID: "merch_x",
		Scope:      merchant.APIKeyScopeRead,
		AllowedIPs: []string{"203.0.113.10"},
	}}, []string{"127.0.0.1/32"})
	h := m.RequireScope(merchant.APIKeyScopeRead, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	token := base64.StdEncoding.EncodeToString([]byte("rzp_test_x:secret"))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic "+token)
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.RemoteAddr = "127.0.0.1:4321"

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}
