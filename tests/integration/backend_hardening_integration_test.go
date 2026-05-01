//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/auth"
	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/gateway"
	"github.com/sanskarpan/PayGate/internal/idempotency"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/order"
	"github.com/sanskarpan/PayGate/internal/outbox"
	"github.com/sanskarpan/PayGate/internal/payment"
)

func TestIntegrationIdempotentOrderCreateReplaysResponse(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, _, _ := buildGatewayMux(db)
	ctx := context.Background()
	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name:         "Replay Merchant",
		Email:        "replay@test.com",
		BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	key, err := merchantSvc.CreateAPIKey(ctx, createdMerchant.ID, merchant.CreateAPIKeyInput{
		Mode:  merchant.APIKeyModeTest,
		Scope: merchant.APIKeyScopeWrite,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	body := []byte(`{"amount":5000,"currency":"INR","receipt":"idem-order"}`)
	first := httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewReader(body))
	first.Header.Set("Authorization", basicAuth(key.KeyID, key.KeySecret))
	first.Header.Set("Idempotency-Key", "order-create-1")
	first.Header.Set("Content-Type", "application/json")
	firstRecorder := httptest.NewRecorder()
	mux.ServeHTTP(firstRecorder, first)
	if firstRecorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", firstRecorder.Code, firstRecorder.Body.String())
	}

	second := httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewReader(body))
	second.Header.Set("Authorization", basicAuth(key.KeyID, key.KeySecret))
	second.Header.Set("Idempotency-Key", "order-create-1")
	second.Header.Set("Content-Type", "application/json")
	secondRecorder := httptest.NewRecorder()
	mux.ServeHTTP(secondRecorder, second)
	if secondRecorder.Code != http.StatusCreated {
		t.Fatalf("expected replay status 201, got %d body=%s", secondRecorder.Code, secondRecorder.Body.String())
	}
	if secondRecorder.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("expected replay header, got %q", secondRecorder.Header().Get("Idempotent-Replayed"))
	}

	var firstBody map[string]any
	var secondBody map[string]any
	if err := json.Unmarshal(firstRecorder.Body.Bytes(), &firstBody); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if err := json.Unmarshal(secondRecorder.Body.Bytes(), &secondBody); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if firstBody["id"] != secondBody["id"] {
		t.Fatalf("expected same order id on replay, got %v vs %v", firstBody["id"], secondBody["id"])
	}
}

func TestIntegrationDashboardSessionAuthAndAPIKeyManagement(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	mux, merchantSvc, orderSvc, _ := buildGatewayMux(db)
	ctx := context.Background()
	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name:         "Dashboard Merchant",
		Email:        "dashboard@test.com",
		BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	if _, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: createdMerchant.ID,
		Amount:     7000,
		Currency:   "INR",
		Receipt:    "dash-order",
	}); err != nil {
		t.Fatalf("create order: %v", err)
	}

	bootstrapReq := httptest.NewRequest(http.MethodPost, "/v1/merchants/"+createdMerchant.ID+"/users/bootstrap", bytes.NewReader([]byte(`{"email":"owner@dashboard.test","password":"supersecure"}`)))
	bootstrapReq.Header.Set("Content-Type", "application/json")
	bootstrapRecorder := httptest.NewRecorder()
	mux.ServeHTTP(bootstrapRecorder, bootstrapReq)
	if bootstrapRecorder.Code != http.StatusCreated {
		t.Fatalf("expected bootstrap 201, got %d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/v1/dashboard/login", bytes.NewReader([]byte(`{"merchant_id":"`+createdMerchant.ID+`","email":"owner@dashboard.test","password":"supersecure"}`)))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRecorder := httptest.NewRecorder()
	mux.ServeHTTP(loginRecorder, loginReq)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
	cookies := loginRecorder.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie after login")
	}

	orderReq := httptest.NewRequest(http.MethodGet, "/v1/orders", nil)
	orderReq.AddCookie(cookies[0])
	orderRecorder := httptest.NewRecorder()
	mux.ServeHTTP(orderRecorder, orderReq)
	if orderRecorder.Code != http.StatusOK {
		t.Fatalf("expected order list 200, got %d body=%s", orderRecorder.Code, orderRecorder.Body.String())
	}

	createKeyReq := httptest.NewRequest(http.MethodPost, "/v1/merchants/me/api-keys", bytes.NewReader([]byte(`{"mode":"test","scope":"write"}`)))
	createKeyReq.Header.Set("Content-Type", "application/json")
	createKeyReq.AddCookie(cookies[0])
	createKeyRecorder := httptest.NewRecorder()
	mux.ServeHTTP(createKeyRecorder, createKeyReq)
	if createKeyRecorder.Code != http.StatusCreated {
		t.Fatalf("expected api key create 201, got %d body=%s", createKeyRecorder.Code, createKeyRecorder.Body.String())
	}

	keyListReq := httptest.NewRequest(http.MethodGet, "/v1/merchants/me/api-keys", nil)
	keyListReq.AddCookie(cookies[0])
	keyListRecorder := httptest.NewRecorder()
	mux.ServeHTTP(keyListRecorder, keyListReq)
	if keyListRecorder.Code != http.StatusOK {
		t.Fatalf("expected key list 200, got %d body=%s", keyListRecorder.Code, keyListRecorder.Body.String())
	}
}

func TestIntegrationExpireAuthorizationWindowMarksOrderFailedAndWritesEvent(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	_, merchantSvc, orderSvc, paymentSvc := buildGatewayMux(db)
	createdMerchant, err := merchantSvc.CreateMerchant(ctx, merchant.CreateMerchantInput{
		Name:         "Expiry Merchant",
		Email:        "expiry@test.com",
		BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	createdOrder, err := orderSvc.Create(ctx, order.CreateInput{
		MerchantID: createdMerchant.ID,
		Amount:     9900,
		Currency:   "INR",
		Receipt:    "expiry-order",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	authorized, err := paymentSvc.Authorize(ctx, payment.AuthorizeInput{
		MerchantID: createdMerchant.ID,
		OrderID:    createdOrder.ID,
		Amount:     createdOrder.Amount,
		Currency:   createdOrder.Currency,
		Method:     "card",
	})
	if err != nil {
		t.Fatalf("authorize payment: %v", err)
	}

	if _, err := db.Exec(ctx, `UPDATE paygate_payments.payments SET authorized_at = NOW() - INTERVAL '6 days' WHERE id = $1`, authorized.PaymentID); err != nil {
		t.Fatalf("age payment auth window: %v", err)
	}
	repo := payment.NewPostgresRepository(db, ledger.NewService(ledger.NewRepository(db)), orderSvc)
	expired, err := repo.ExpireAuthorizationWindow(ctx, 5*24*time.Hour)
	if err != nil {
		t.Fatalf("expire authorization window: %v", err)
	}
	if expired != 1 {
		t.Fatalf("expected 1 expired payment, got %d", expired)
	}

	var paymentStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM paygate_payments.payments WHERE id = $1`, authorized.PaymentID).Scan(&paymentStatus); err != nil {
		t.Fatalf("query payment status: %v", err)
	}
	if paymentStatus != string(payment.StateAutoRefunded) {
		t.Fatalf("expected auto_refunded payment, got %s", paymentStatus)
	}

	var orderStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM paygate_orders.orders WHERE id = $1`, createdOrder.ID).Scan(&orderStatus); err != nil {
		t.Fatalf("query order status: %v", err)
	}
	if orderStatus != "failed" {
		t.Fatalf("expected failed order, got %s", orderStatus)
	}

	var count int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM public.outbox WHERE aggregate_id = $1 AND event_type = 'payment.auto_refunded'`, authorized.PaymentID).Scan(&count); err != nil {
		t.Fatalf("query outbox auto refunded event: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one payment.auto_refunded event, got %d", count)
	}
}

func TestIntegrationOutboxRelayPublishesAndMarksRows(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	// Clear any unpublished outbox rows left by prior tests in this run so
	// that the relay publish count assertion below is deterministic.
	if _, err := db.Exec(ctx, `DELETE FROM public.outbox WHERE published_at IS NULL`); err != nil {
		t.Fatalf("clear unpublished outbox rows: %v", err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	writer := outbox.NewWriter()
	if err := writer.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "payment",
		AggregateID:   "pay_test_outbox",
		EventType:     "payment.captured",
		MerchantID:    "merch_outbox",
		Payload:       map[string]any{"payment_id": "pay_test_outbox"},
	}); err != nil {
		t.Fatalf("write outbox tx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit outbox tx: %v", err)
	}

	publisher := &fakePublisher{}
	relay := outbox.NewRelay(db, publisher, time.Second, nil)
	published, err := relay.PublishBatch(ctx, 10)
	if err != nil {
		t.Fatalf("publish outbox batch: %v", err)
	}
	if published != 1 {
		t.Fatalf("expected one published event, got %d", published)
	}
	if len(publisher.topics) != 1 || publisher.topics[0] != "paygate.payments" {
		t.Fatalf("expected paygate.payments topic, got %#v", publisher.topics)
	}

	var publishedAt *time.Time
	if err := db.QueryRow(ctx, `SELECT published_at FROM public.outbox WHERE aggregate_id = $1`, "pay_test_outbox").Scan(&publishedAt); err != nil {
		t.Fatalf("query published_at: %v", err)
	}
	if publishedAt == nil {
		t.Fatal("expected published_at to be set")
	}
}

func buildGatewayMux(db *pgxpool.Pool) (*http.ServeMux, *merchant.Service, *order.Service, *payment.Service) {
	merchantRepo := merchant.NewPostgresRepository(db)
	merchantSvc := merchant.NewService(merchantRepo, merchant.WithSessionSecret("integration-dashboard-secret"))
	authMw := auth.NewMiddleware(merchantSvc)
	idemMw := idempotency.NewMiddleware(idempotency.NewStore(db, nil))

	orderSvc := order.NewService(order.NewPostgresRepository(db))
	ledgerSvc := ledger.NewService(ledger.NewRepository(db))
	paymentSvc := payment.NewService(payment.NewPostgresRepository(db, ledgerSvc, orderSvc), gateway.NewSimulator())

	merchantHandler := merchant.NewHandler(merchantSvc)
	orderHandler := order.NewHandler(orderSvc)
	paymentHandler := payment.NewHandler(paymentSvc)

	protected := func(scope merchant.APIKeyScope, next http.Handler) http.Handler {
		return authMw.RequireScope(scope, idemMw.Wrap(next))
	}

	mux := http.NewServeMux()
	merchantHandler.RegisterRoutes(mux)
	merchantHandler.RegisterProtectedRoutes(mux, protected)
	orderHandler.RegisterRoutesWithAuth(mux, protected)
	paymentHandler.RegisterRoutesWithAuth(mux, protected)
	mux.Handle("GET /v1/merchants/me", authMw.RequireScope(merchant.APIKeyScopeRead, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := httpx.PrincipalFromContext(r.Context())
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"merchant_id": p.MerchantID})
	})))
	return mux, merchantSvc, orderSvc, paymentSvc
}

type fakePublisher struct {
	topics []string
	keys   []string
	body   [][]byte
}

func (f *fakePublisher) Publish(_ context.Context, topic string, key string, payload []byte) error {
	f.topics = append(f.topics, topic)
	f.keys = append(f.keys, key)
	f.body = append(f.body, append([]byte(nil), payload...))
	return nil
}

func (f *fakePublisher) Close() error { return nil }

func basicAuth(id, secret string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(id+":"+secret))
}
