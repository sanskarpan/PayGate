package order

import "context"

type ListFilter struct {
	MerchantID string
	Count      int
	From       int64
	To         int64
	Cursor     string
}

type ListResult struct {
	Items      []Order
	HasMore    bool
	NextCursor string
}

type Repository interface {
	Create(ctx context.Context, order Order) (Order, error)
	GetByID(ctx context.Context, merchantID, orderID string) (Order, error)
	List(ctx context.Context, f ListFilter) (ListResult, error)
	ExpireDueOrders(ctx context.Context) (int64, error)
	MarkOrderPaid(ctx context.Context, merchantID, orderID string) error
}
