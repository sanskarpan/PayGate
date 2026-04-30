package merchant

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
