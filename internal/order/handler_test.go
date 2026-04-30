package order

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
)

type fakeRepo struct {
	orders map[string]Order
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{orders: map[string]Order{}}
}

func (f *fakeRepo) Create(_ context.Context, o Order) (Order, error) {
	o.CreatedAt = time.Now().UTC()
	o.UpdatedAt = o.CreatedAt
	f.orders[o.ID] = o
	return o, nil
}

func (f *fakeRepo) GetByID(_ context.Context, merchantID, orderID string) (Order, error) {
	o, ok := f.orders[orderID]
	if !ok || o.MerchantID != merchantID {
		return Order{}, ErrOrderNotFound
	}
	return o, nil
}

func (f *fakeRepo) List(_ context.Context, _ ListFilter) (ListResult, error) {
	items := make([]Order, 0, len(f.orders))
	for _, o := range f.orders {
		items = append(items, o)
	}
	return ListResult{Items: items, HasMore: false}, nil
}

func (f *fakeRepo) ExpireDueOrders(context.Context) (int64, error)      { return 0, nil }
func (f *fakeRepo) MarkOrderPaid(context.Context, string, string) error { return nil }

func TestCreateOrder(t *testing.T) {
	repo := newFakeRepo()
	h := NewHandler(NewService(repo))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]any{
		"amount":   50000,
		"currency": "INR",
		"receipt":  "rcpt_1",
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewReader(body))
	req = req.WithContext(httpx.WithPrincipal(req.Context(), httpx.Principal{MerchantID: "merch_123", Scope: "write"}))
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
}
