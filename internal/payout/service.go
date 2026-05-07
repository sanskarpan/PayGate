package payout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Service orchestrates payout use-cases.
type Service struct {
	repo   Repository
	logger *slog.Logger
}

// NewService creates a new Service.
func NewService(repo Repository, logger *slog.Logger) *Service {
	return &Service{repo: repo, logger: logger}
}

// InitiatePayout creates a payout for the given settlement if one does not already exist,
// transitions it to processing, and simulates the bank transfer asynchronously.
func (s *Service) InitiatePayout(ctx context.Context, merchantID, settlementID string) (Payout, error) {
	// Get or create the payout for this settlement.
	p, err := s.repo.GetBySettlementID(ctx, merchantID, settlementID)
	if err != nil {
		if !errors.Is(err, ErrPayoutNotFound) {
			return Payout{}, err
		}
		// Payout does not exist yet — we need to look up the settlement net amount.
		// The caller (handler) is expected to pass amount/currency via context or the
		// settlement service. Here we delegate to GetBySettlementID callers' responsibility:
		// if none found, return the not found error so the handler can supply the amount.
		return Payout{}, ErrPayoutNotFound
	}

	// Initiate the existing payout.
	p, err = s.repo.Initiate(ctx, merchantID, p.ID)
	if err != nil {
		return Payout{}, err
	}

	// Detach from request context so HTTP cancellation doesn't abort the transfer.
	detached := context.WithoutCancel(ctx)
	go s.simulateBankTransfer(detached, p.MerchantID, p.ID)

	return p, nil
}

// InitiatePayoutForSettlement creates a payout for the given settlement using the provided
// amount and currency, then transitions it to processing and kicks off a simulated bank transfer.
func (s *Service) InitiatePayoutForSettlement(ctx context.Context, merchantID, settlementID string, amount int64, currency string) (Payout, error) {
	// Try to get existing payout first.
	existing, err := s.repo.GetBySettlementID(ctx, merchantID, settlementID)
	if err != nil && !errors.Is(err, ErrPayoutNotFound) {
		return Payout{}, err
	}

	var p Payout
	if errors.Is(err, ErrPayoutNotFound) {
		// Create a new payout.
		p, err = s.repo.CreateForSettlement(ctx, merchantID, settlementID, amount, currency)
		if err != nil {
			return Payout{}, fmt.Errorf("create payout: %w", err)
		}
	} else {
		p = existing
	}

	// Initiate (pending → processing).
	p, err = s.repo.Initiate(ctx, merchantID, p.ID)
	if err != nil {
		return Payout{}, fmt.Errorf("initiate payout: %w", err)
	}

	// Detach from request context so HTTP cancellation doesn't abort the transfer.
	detached := context.WithoutCancel(ctx)
	go s.simulateBankTransfer(detached, p.MerchantID, p.ID)

	return p, nil
}

// simulateBankTransfer sleeps 2s then completes (or fails) the payout.
func (s *Service) simulateBankTransfer(ctx context.Context, merchantID, payoutID string) {
	time.Sleep(2 * time.Second)

	bankRef := fmt.Sprintf("BNK_%d", time.Now().UnixNano())
	detached := ctx

	if _, err := s.repo.Complete(detached, merchantID, payoutID, bankRef); err != nil {
		s.logger.Error("simulate bank transfer: complete failed",
			"payout_id", payoutID,
			"merchant_id", merchantID,
			"error", err,
		)
		if _, ferr := s.repo.Fail(detached, merchantID, payoutID, err.Error()); ferr != nil {
			s.logger.Error("simulate bank transfer: fail also failed",
				"payout_id", payoutID,
				"error", ferr,
			)
		}
	}
}

// Get returns a payout by ID scoped to the merchant.
func (s *Service) Get(ctx context.Context, merchantID, id string) (Payout, error) {
	return s.repo.GetByID(ctx, merchantID, id)
}

// List returns payouts for a merchant, most recent first up to limit.
func (s *Service) List(ctx context.Context, merchantID string, limit int) ([]Payout, error) {
	return s.repo.List(ctx, merchantID, limit)
}

// GetBySettlement returns the payout associated with a settlement.
func (s *Service) GetBySettlement(ctx context.Context, merchantID, settlementID string) (Payout, error) {
	return s.repo.GetBySettlementID(ctx, merchantID, settlementID)
}
