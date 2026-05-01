//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sanskarpan/PayGate/internal/merchant"
)

func TestIntegrationTeamInviteAndAccept(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()

	// Create a merchant and a dashboard admin user.
	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "RBAC Merchant", Email: "rbac@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	adminUser, err := merchantSvc.BootstrapMerchantUser(ctx, m.ID, merchant.BootstrapMerchantUserInput{
		Email: "admin@rbac.com", Password: "secret1234",
	})
	if err != nil {
		t.Fatalf("bootstrap admin user: %v", err)
	}

	// Log in to get a session cookie.
	loginBody, _ := json.Marshal(map[string]string{
		"merchant_id": m.ID,
		"email":       adminUser.Email,
		"password":    "secret1234",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/v1/dashboard/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == merchant.DashboardSessionCookieName {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie after login")
	}

	// Invite a developer.
	invBody, _ := json.Marshal(map[string]string{
		"email": "dev@rbac.com",
		"role":  "developer",
	})
	invReq := httptest.NewRequest(http.MethodPost, "/v1/merchants/me/invitations", bytes.NewReader(invBody))
	invReq.Header.Set("Content-Type", "application/json")
	invReq.AddCookie(sessionCookie)
	invRec := httptest.NewRecorder()
	mux.ServeHTTP(invRec, invReq)
	if invRec.Code != http.StatusCreated {
		t.Fatalf("invite user: expected 201, got %d body=%s", invRec.Code, invRec.Body.String())
	}

	var invResp map[string]any
	if err := json.Unmarshal(invRec.Body.Bytes(), &invResp); err != nil {
		t.Fatalf("decode invite response: %v", err)
	}
	token, ok := invResp["token"].(string)
	if !ok || token == "" {
		t.Fatal("expected token in invite response")
	}

	// List invitations — should see the pending one.
	listReq := httptest.NewRequest(http.MethodGet, "/v1/merchants/me/invitations", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list invitations: expected 200, got %d", listRec.Code)
	}
	var listResp map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if int(listResp["count"].(float64)) < 1 {
		t.Fatal("expected at least one invitation")
	}

	// Accept the invitation as the invited developer.
	acceptBody, _ := json.Marshal(map[string]string{
		"token":    token,
		"password": "devpassword99",
	})
	acceptReq := httptest.NewRequest(http.MethodPost, "/v1/dashboard/accept-invitation", bytes.NewReader(acceptBody))
	acceptReq.Header.Set("Content-Type", "application/json")
	acceptRec := httptest.NewRecorder()
	mux.ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusCreated {
		t.Fatalf("accept invitation: expected 201, got %d body=%s", acceptRec.Code, acceptRec.Body.String())
	}

	var acceptResp map[string]any
	if err := json.Unmarshal(acceptRec.Body.Bytes(), &acceptResp); err != nil {
		t.Fatalf("decode accept response: %v", err)
	}
	if acceptResp["role"] != "developer" {
		t.Fatalf("expected developer role, got %v", acceptResp["role"])
	}
	if acceptResp["merchant_id"] != m.ID {
		t.Fatalf("expected merchant %s, got %v", m.ID, acceptResp["merchant_id"])
	}

	// Accepting the same token a second time should fail.
	acceptReq2 := httptest.NewRequest(http.MethodPost, "/v1/dashboard/accept-invitation", bytes.NewReader(acceptBody))
	acceptReq2.Header.Set("Content-Type", "application/json")
	acceptRec2 := httptest.NewRecorder()
	mux.ServeHTTP(acceptRec2, acceptReq2)
	if acceptRec2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on reuse, got %d body=%s", acceptRec2.Code, acceptRec2.Body.String())
	}
}

func TestIntegrationAPIKeyIPAllowlist(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "IP Allowlist Merchant", Email: "ipallowlist@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, m.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeAdmin,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	auth := basicAuth(key.KeyID, key.KeySecret)

	// Restrict the key to an IP that won't match the test request (127.0.0.1 is the test remote addr).
	restrictBody, _ := json.Marshal(map[string]any{
		"allowed_ips": []string{"203.0.113.1"},
	})
	restrictReq := httptest.NewRequest(http.MethodPut, "/v1/merchants/me/api-keys/"+key.KeyID+"/allowed-ips", bytes.NewReader(restrictBody))
	restrictReq.Header.Set("Content-Type", "application/json")
	restrictReq.Header.Set("Authorization", auth)
	restrictRec := httptest.NewRecorder()
	mux.ServeHTTP(restrictRec, restrictReq)
	if restrictRec.Code != http.StatusOK {
		t.Fatalf("restrict key: expected 200, got %d body=%s", restrictRec.Code, restrictRec.Body.String())
	}

	// A subsequent request with the same key from a non-allowed IP should be forbidden.
	blockedReq := httptest.NewRequest(http.MethodGet, "/v1/merchants/me/api-keys", nil)
	blockedReq.Header.Set("Authorization", auth)
	blockedReq.RemoteAddr = "192.168.1.100:12345" // not in allowed_ips
	blockedRec := httptest.NewRecorder()
	mux.ServeHTTP(blockedRec, blockedReq)
	if blockedRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for blocked IP, got %d body=%s", blockedRec.Code, blockedRec.Body.String())
	}

	// Reset the allowlist to empty (no restriction) — request from any IP should succeed.
	resetBody, _ := json.Marshal(map[string]any{"allowed_ips": []string{}})
	// Use a direct service call to reset since the HTTP key is now blocked from any source.
	repo := merchant.NewPostgresRepository(db)
	if err := repo.UpdateAPIKeyAllowedIPs(ctx, m.ID, key.KeyID, []string{}); err != nil {
		t.Fatalf("reset allowed ips: %v", err)
	}

	// Now the request should succeed regardless of source IP.
	openReq := httptest.NewRequest(http.MethodGet, "/v1/merchants/me/api-keys", nil)
	openReq.Header.Set("Authorization", auth)
	openReq.RemoteAddr = "192.168.99.99:1234"
	openRec := httptest.NewRecorder()
	mux.ServeHTTP(openRec, openReq)
	if openRec.Code != http.StatusOK {
		t.Fatalf("expected 200 after removing allowlist, got %d body=%s", openRec.Code, openRec.Body.String())
	}
	_ = resetBody
}
