//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func createCardTokenViaMux(t *testing.T, mux *http.ServeMux, authHeader string, reusable bool) string {
	t.Helper()
	expires := time.Now().UTC().AddDate(3, 0, 0)
	body, err := json.Marshal(map[string]any{
		"card_number": "4111111111111111",
		"exp_month":   int(expires.Month()),
		"exp_year":    expires.Year(),
		"reusable":    reusable,
	})
	if err != nil {
		t.Fatalf("marshal card token request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/card-tokens", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create card token: expected 201 got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode card token response: %v", err)
	}
	id, ok := resp["id"].(string)
	if !ok || id == "" {
		t.Fatalf("expected token id in response, got %#v", resp)
	}
	return id
}
