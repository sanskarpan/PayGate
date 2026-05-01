//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/audit"
	"github.com/sanskarpan/PayGate/internal/auth"
	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/idempotency"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
)

func buildAuditMux(db *pgxpool.Pool) (*http.ServeMux, *merchant.Service, *audit.Service) {
	merchantRepo := merchant.NewPostgresRepository(db)
	merchantSvc := merchant.NewService(merchantRepo, merchant.WithSessionSecret("audit-test-secret"))
	authMw := auth.NewMiddleware(merchantSvc)
	idemMw := idempotency.NewMiddleware(idempotency.NewStore(db, nil))

	auditRepo := audit.NewPostgresRepository(db)
	auditSvc := audit.NewService(auditRepo, nil)
	auditHandler := audit.NewHandler(auditSvc)

	orderSvc := order.NewService(order.NewPostgresRepository(db))
	orderHandler := order.NewHandler(orderSvc)
	merchantHandler := merchant.NewHandler(merchantSvc)

	protected := func(scope merchant.APIKeyScope, next http.Handler) http.Handler {
		return authMw.RequireScope(scope, idemMw.Wrap(auditSvc.Middleware(next)))
	}

	mux := http.NewServeMux()
	merchantHandler.RegisterRoutes(mux)
	merchantHandler.RegisterProtectedRoutes(mux, protected)
	orderHandler.RegisterRoutesWithAuth(mux, protected)
	auditHandler.RegisterRoutesWithAuth(mux, func(next http.Handler) http.Handler {
		return authMw.RequireScope(merchant.APIKeyScopeRead, next)
	})
	mux.Handle("GET /v1/merchants/me", authMw.RequireScope(merchant.APIKeyScopeRead, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := httpx.PrincipalFromContext(r.Context())
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"merchant_id": p.MerchantID})
	})))
	return mux, merchantSvc, auditSvc
}

func TestIntegrationAuditLogRecordsOrderCreate(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _ := buildAuditMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Audit Merchant", Email: "audit@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, m.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeWrite,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHdr := basicAuth(key.KeyID, key.KeySecret)

	// Create an order — should produce an audit log entry.
	body, _ := json.Marshal(map[string]any{"amount": 5000, "currency": "INR", "receipt": "audit-order-1"})
	req := httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewReader(body))
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", "corr-audit-001")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create order: expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Give the goroutine time to write.
	time.Sleep(100 * time.Millisecond)

	// Query audit logs via the API.
	logReq := httptest.NewRequest(http.MethodGet, "/v1/audit-logs?resource_type=orders", nil)
	logReq.Header.Set("Authorization", authHdr)
	logRec := httptest.NewRecorder()
	mux.ServeHTTP(logRec, logReq)
	if logRec.Code != http.StatusOK {
		t.Fatalf("list audit logs: expected 200, got %d body=%s", logRec.Code, logRec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(logRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode audit list: %v", err)
	}
	if int(resp["count"].(float64)) < 1 {
		t.Fatal("expected at least one audit log for orders")
	}
	items := resp["items"].([]any)
	entry := items[0].(map[string]any)
	if entry["resource_type"] != "orders" {
		t.Fatalf("expected resource_type=orders, got %v", entry["resource_type"])
	}
	if entry["correlation_id"] != "corr-audit-001" {
		t.Fatalf("expected correlation_id=corr-audit-001, got %v", entry["correlation_id"])
	}
}

func TestIntegrationAuditLogFiltersOutGetRequests(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, auditSvc := buildAuditMux(db)
	ctx := context.Background()

	m, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name: "Audit Read Merchant", Email: "auditread@test.com", BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, m.ID, merchant.CreateAPIKeyInput{
		Mode: merchant.APIKeyModeTest, Scope: merchant.APIKeyScopeRead,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	authHdr := basicAuth(key.KeyID, key.KeySecret)

	// Count audit logs before the GET request.
	beforeLogs, _ := auditSvc.List(ctx, audit.ListInput{MerchantID: m.ID})
	before := len(beforeLogs)

	// GET /v1/merchants/me — should NOT create an audit entry.
	req := httptest.NewRequest(http.MethodGet, "/v1/merchants/me", nil)
	req.Header.Set("Authorization", authHdr)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET merchants/me: expected 200, got %d", rec.Code)
	}

	time.Sleep(50 * time.Millisecond)

	afterLogs, _ := auditSvc.List(ctx, audit.ListInput{MerchantID: m.ID})
	if len(afterLogs) != before {
		t.Fatalf("GET request should not create audit log: before=%d after=%d", before, len(afterLogs))
	}
}
