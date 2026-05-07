package payout

import "context"

// Repository defines storage operations for the payout service.
type Repository interface {
	// CreateForSettlement creates a new payout in pending state for the given settlement.
	CreateForSettlement(ctx context.Context, merchantID, settlementID string, amount int64, currency string) (Payout, error)

	// GetByID returns a payout by ID scoped to the merchant.
	GetByID(ctx context.Context, merchantID, id string) (Payout, error)

	// GetBySettlementID returns the payout for a settlement scoped to the merchant.
	GetBySettlementID(ctx context.Context, merchantID, settlementID string) (Payout, error)

	// List returns the most recent payouts for a merchant up to limit.
	List(ctx context.Context, merchantID string, limit int) ([]Payout, error)

	// Initiate transitions a payout from pending to processing.
	Initiate(ctx context.Context, merchantID, id string) (Payout, error)

	// Complete transitions a payout from processing to completed and records the bank reference.
	Complete(ctx context.Context, merchantID, id, bankReference string) (Payout, error)

	// Fail transitions a payout from processing to failed and records the failure reason.
	Fail(ctx context.Context, merchantID, id, reason string) (Payout, error)
}
