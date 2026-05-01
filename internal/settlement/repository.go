package settlement

import (
	"context"
	"time"
)

// Repository defines storage operations for the settlement service.
type Repository interface {
	// RunBatch collects eligible payments for merchantID in [periodStart, periodEnd),
	// creates a Settlement + SettlementItems, writes ledger entries and outbox events,
	// and marks the payments as settled — all in a single Postgres transaction.
	RunBatch(ctx context.Context, merchantID string, periodStart, periodEnd time.Time) (Settlement, error)

	// GetSettlement returns a settlement by ID scoped to the merchant.
	GetSettlement(ctx context.Context, merchantID, id string) (Settlement, error)

	// ListSettlements returns all settlements for a merchant, most recent first.
	ListSettlements(ctx context.Context, merchantID string) ([]Settlement, error)

	// GetSettlementItems returns all items for a settlement.
	GetSettlementItems(ctx context.Context, settlementID string) ([]SettlementItem, error)
}
