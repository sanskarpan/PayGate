package merchant

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateMerchantHandler(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	h := NewHandler(svc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]any{
		"name":          "Acme",
		"email":         "owner@acme.com",
		"business_type": "company",
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/merchants", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateAPIKeyRequiresBootstrapOrAdmin(t *testing.T) {
	repo := newFakeRepo()
	repo.merchants["merch_1"] = Merchant{ID: "merch_1", Name: "Acme", Email: "owner@acme.com", BusinessType: "company", Status: MerchantStatusActive}
	repo.keys["rzp_test_existing"] = APIKey{ID: "rzp_test_existing", MerchantID: "merch_1", Status: APIKeyStatusActive}
	svc := NewService(repo)
	h := NewHandler(svc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]any{"mode": "test", "scope": "write"})
	req := httptest.NewRequest(http.MethodPost, "/v1/merchants/merch_1/keys", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateAPIKeyAllowsAdminAuth(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	m, err := svc.CreateMerchant(context.Background(), CreateMerchantInput{Name: "Acme", Email: "owner@acme.com", BusinessType: "company"})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	adminKey, err := svc.CreateAPIKey(context.Background(), m.ID, CreateAPIKeyInput{Mode: APIKeyModeTest, Scope: APIKeyScopeAdmin})
	if err != nil {
		t.Fatalf("create admin key: %v", err)
	}
	h := NewHandler(svc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]any{"mode": "test", "scope": "write"})
	req := httptest.NewRequest(http.MethodPost, "/v1/merchants/"+m.ID+"/keys", bytes.NewReader(body))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(adminKey.KeyID+":"+adminKey.KeySecret)))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestDashboardLoginRejectsUnsafeRedirectAndSetsSecureCookieForHTTPS(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, WithSessionSecret("abcdefghijklmnopqrstuvwxyz012345"))
	merchantRecord, err := svc.CreateMerchant(context.Background(), CreateMerchantInput{Name: "Acme", Email: "owner@acme.com", BusinessType: "company"})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	if _, err := svc.BootstrapMerchantUser(context.Background(), merchantRecord.ID, BootstrapMerchantUserInput{
		Email:    "owner@acme.com",
		Password: "strong-password-123",
	}); err != nil {
		t.Fatalf("bootstrap user: %v", err)
	}
	h := NewHandler(svc, WithDashboardOrigin("https://dashboard.paygate.test"))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	form := "merchant_id=" + merchantRecord.ID + "&email=owner%40acme.com&password=strong-password-123&redirect_to=https://evil.example"
	req := httptest.NewRequest(http.MethodPost, "/v1/dashboard/login", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Location"); got != "https://dashboard.paygate.test/orders" {
		t.Fatalf("expected fallback redirect, got %q", got)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie to be set")
	}
	if !cookies[0].Secure {
		t.Fatal("expected dashboard session cookie to be secure for https requests")
	}
}

func TestDashboardLogoutRejectsUnsafeRedirect(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	h := NewHandler(svc, WithDashboardOrigin("https://dashboard.paygate.test"))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/v1/dashboard/logout?redirect_to=//evil.example", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rr.Code)
	}
	if got := rr.Header().Get("Location"); got != "https://dashboard.paygate.test/" {
		t.Fatalf("expected safe fallback redirect, got %q", got)
	}
}

func TestDashboardLoginAllowsConfiguredAbsoluteRedirect(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, WithSessionSecret("abcdefghijklmnopqrstuvwxyz012345"))
	merchantRecord, err := svc.CreateMerchant(context.Background(), CreateMerchantInput{Name: "Acme", Email: "owner@acme.com", BusinessType: "company"})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	if _, err := svc.BootstrapMerchantUser(context.Background(), merchantRecord.ID, BootstrapMerchantUserInput{
		Email:    "owner@acme.com",
		Password: "strong-password-123",
	}); err != nil {
		t.Fatalf("bootstrap user: %v", err)
	}
	h := NewHandler(svc, WithDashboardOrigin("http://127.0.0.1:33001"))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	form := "merchant_id=" + merchantRecord.ID + "&email=owner%40acme.com&password=strong-password-123&redirect_to=http%3A%2F%2F127.0.0.1%3A33001%2Foverview"
	req := httptest.NewRequest(http.MethodPost, "/v1/dashboard/login", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Location"); got != "http://127.0.0.1:33001/overview" {
		t.Fatalf("expected configured absolute redirect, got %q", got)
	}
}
