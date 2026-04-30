package order

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

var ErrOrderNotFound = errors.New("order not found")

type Service struct {
	repo Repository
}

type CreateInput struct {
	MerchantID     string         `json:"-"`
	IdempotencyKey string         `json:"-"`
	Amount         int64          `json:"amount"`
	Currency       string         `json:"currency"`
	Receipt        string         `json:"receipt"`
	Notes          map[string]any `json:"notes"`
	PartialPayment bool           `json:"partial_payment"`
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Order, error) {
	currency := strings.TrimSpace(strings.ToUpper(in.Currency))
	if currency == "" {
		currency = "INR"
	}

	o := Order{
		ID:             idgen.New("order"),
		MerchantID:     strings.TrimSpace(in.MerchantID),
		IdempotencyKey: strings.TrimSpace(in.IdempotencyKey),
		Amount:         in.Amount,
		AmountPaid:     0,
		AmountDue:      in.Amount,
		Currency:       currency,
		Receipt:        strings.TrimSpace(in.Receipt),
		Status:         StateCreated,
		PartialPayment: in.PartialPayment,
		Notes:          in.Notes,
		ExpiresAt:      time.Now().UTC().Add(30 * time.Minute),
	}

	if err := o.ValidateForCreate(); err != nil {
		return Order{}, err
	}

	return s.repo.Create(ctx, o)
}

func (s *Service) GetByID(ctx context.Context, merchantID, orderID string) (Order, error) {
	return s.repo.GetByID(ctx, merchantID, orderID)
}

func (s *Service) List(ctx context.Context, f ListFilter) (ListResult, error) {
	if f.Count <= 0 || f.Count > 100 {
		f.Count = 10
	}
	return s.repo.List(ctx, f)
}

func (s *Service) MarkPaid(ctx context.Context, merchantID, orderID string) error {
	return s.repo.MarkOrderPaid(ctx, merchantID, orderID)
}
