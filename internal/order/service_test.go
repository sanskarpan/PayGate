package order

import (
	"context"
	"testing"
)

type listFilterRepo struct {
	lastFilter ListFilter
}

func (r *listFilterRepo) Create(context.Context, Order) (Order, error)           { return Order{}, nil }
func (r *listFilterRepo) GetByID(context.Context, string, string) (Order, error) { return Order{}, nil }
func (r *listFilterRepo) ExpireDueOrders(context.Context) (int64, error)         { return 0, nil }
func (r *listFilterRepo) MarkOrderPaid(context.Context, string, string) error    { return nil }
func (r *listFilterRepo) List(_ context.Context, f ListFilter) (ListResult, error) {
	r.lastFilter = f
	return ListResult{}, nil
}

func TestListAppliesDefaultPagination(t *testing.T) {
	repo := &listFilterRepo{}
	svc := NewService(repo)
	_, _ = svc.List(context.Background(), ListFilter{MerchantID: "m1", Count: 0})
	if repo.lastFilter.Count != 10 {
		t.Fatalf("expected default count=10, got %d", repo.lastFilter.Count)
	}
}
