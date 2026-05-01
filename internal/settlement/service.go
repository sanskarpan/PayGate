package settlement

import (
	"context"
	"time"
)

// Service orchestrates the settlement use-cases.
type Service struct {
	repo Repository
}

// NewService creates a new Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// RunBatch runs the settlement batch for merchantID covering [periodStart, periodEnd).
func (s *Service) RunBatch(ctx context.Context, merchantID string, periodStart, periodEnd time.Time) (Settlement, error) {
	return s.repo.RunBatch(ctx, merchantID, periodStart, periodEnd)
}

// Get returns a settlement by ID, scoped to the merchant.
func (s *Service) Get(ctx context.Context, merchantID, id string) (Settlement, error) {
	return s.repo.GetSettlement(ctx, merchantID, id)
}

// List returns all settlements for a merchant, most recent first.
func (s *Service) List(ctx context.Context, merchantID string) ([]Settlement, error) {
	return s.repo.ListSettlements(ctx, merchantID)
}

// GetItems returns the settlement items for a settlement.
func (s *Service) GetItems(ctx context.Context, merchantID, settlementID string) (Settlement, []SettlementItem, error) {
	sttl, err := s.repo.GetSettlement(ctx, merchantID, settlementID)
	if err != nil {
		return Settlement{}, nil, err
	}
	items, err := s.repo.GetSettlementItems(ctx, settlementID)
	if err != nil {
		return Settlement{}, nil, err
	}
	return sttl, items, nil
}
