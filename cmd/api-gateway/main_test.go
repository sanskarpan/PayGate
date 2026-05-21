package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sanskarpan/PayGate/internal/eventschema"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/saga"
)

func TestRegisterAdvancedPlatformRoutes(t *testing.T) {
	mux := http.NewServeMux()
	wrap := func(scope merchant.APIKeyScope, next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	}

	registerAdvancedPlatformRoutes(mux, wrap, saga.NewHandler(nil), eventschema.NewHandler(nil), ledger.NewHoldHandler(nil))

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/v1/sagas"},
		{method: http.MethodGet, path: "/v1/sagas/saga_123"},
		{method: http.MethodGet, path: "/v1/event-schemas"},
		{method: http.MethodGet, path: "/v1/event-schemas/payment.captured"},
		{method: http.MethodGet, path: "/v1/event-schema-rollouts/rollout_123"},
		{method: http.MethodGet, path: "/v1/ledger/holds"},
		{method: http.MethodPost, path: "/v1/ledger/holds/hold_123/release"},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s %s: got status %d, want %d", tc.method, tc.path, rec.Code, http.StatusNoContent)
		}
	}
}
