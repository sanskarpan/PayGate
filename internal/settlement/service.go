package settlement

import (
	"context"
	"time"

	"github.com/sanskarpan/PayGate/internal/merchant"
)

// Service orchestrates the settlement use-cases.
type Service struct {
	repo                  Repository
	reservePolicyResolver interface {
		GetReservePolicy(ctx context.Context, merchantID string) (merchant.ReservePolicy, error)
	}
}

// NewService creates a new Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) SetReservePolicyResolver(resolver interface {
	GetReservePolicy(ctx context.Context, merchantID string) (merchant.ReservePolicy, error)
}) {
	s.reservePolicyResolver = resolver
}

// RunBatch runs the settlement batch for merchantID covering [periodStart, periodEnd).
func (s *Service) RunBatch(ctx context.Context, merchantID string, periodStart, periodEnd time.Time) (Settlement, error) {
	return s.repo.RunBatch(ctx, merchantID, periodStart, periodEnd)
}

// RunPartialBatch settles only the given payment IDs.
func (s *Service) RunPartialBatch(ctx context.Context, merchantID string, paymentIDs []string) (Settlement, error) {
	if len(paymentIDs) == 0 {
		return Settlement{}, ErrNoEligiblePayments
	}
	return s.repo.RunPartialBatch(ctx, merchantID, paymentIDs)
}

// Get returns a settlement by ID, scoped to the merchant.
func (s *Service) Get(ctx context.Context, merchantID, id string) (Settlement, error) {
	return s.repo.GetSettlement(ctx, merchantID, id)
}

// List returns all settlements for a merchant, most recent first.
func (s *Service) List(ctx context.Context, merchantID string) ([]Settlement, error) {
	return s.repo.ListSettlements(ctx, merchantID)
}

// Hold places a settlement on hold with the given reason.
func (s *Service) Hold(ctx context.Context, merchantID, settlementID, reason string) error {
	return s.repo.HoldSettlement(ctx, merchantID, settlementID, reason)
}

// Release removes a hold from a settlement.
func (s *Service) Release(ctx context.Context, merchantID, settlementID string) error {
	return s.repo.ReleaseSettlement(ctx, merchantID, settlementID)
}

func (s *Service) MarkRollback(ctx context.Context, merchantID, settlementID, reason string) (Settlement, error) {
	return s.repo.MarkRollback(ctx, merchantID, settlementID, reason)
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

func (s *Service) GetPreferences(ctx context.Context, merchantID string) (Preferences, error) {
	return s.repo.GetPreferences(ctx, merchantID)
}

func (s *Service) UpsertPreferences(ctx context.Context, prefs Preferences) (Preferences, error) {
	return s.repo.UpsertPreferences(ctx, prefs)
}

func (s *Service) GenerateStatement(ctx context.Context, merchantID, settlementID string) (Statement, error) {
	return s.repo.GenerateStatement(ctx, merchantID, settlementID)
}

func (s *Service) GetStatement(ctx context.Context, merchantID, settlementID string) (Statement, error) {
	return s.repo.GetStatement(ctx, merchantID, settlementID)
}

func (s *Service) ListAdjustments(ctx context.Context, merchantID, settlementID string) ([]Adjustment, error) {
	return s.repo.ListAdjustments(ctx, merchantID, settlementID)
}
