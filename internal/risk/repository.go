package risk

import "context"

// Repository defines storage for risk events and velocity counters.
type Repository interface {
	// CreateRiskEvent persists a new risk event and returns it with ID and CreatedAt set.
	CreateRiskEvent(ctx context.Context, ev RiskEvent) (RiskEvent, error)
	// GetRiskEvent returns a risk event by ID.
	GetRiskEvent(ctx context.Context, merchantID, eventID string) (RiskEvent, error)
	// ListRiskEvents returns risk events for a merchant, ordered by created_at desc.
	ListRiskEvents(ctx context.Context, merchantID string, limit int, unresolvedOnly bool) ([]RiskEvent, error)
	// ResolveRiskEvent marks a risk event as resolved.
	ResolveRiskEvent(ctx context.Context, merchantID, eventID, resolvedBy string) error

	// UpsertVelocityCounter increments (or creates) a velocity counter for the given window.
	// Returns the updated count.
	UpsertVelocityCounter(ctx context.Context, dimension, dimValue string, window VelocityWindow, amount int64) (int, error)
	// GetVelocityCount returns the current count for a dimension/value/window.
	GetVelocityCount(ctx context.Context, dimension, dimValue string, window VelocityWindow) (int, error)
	// MerchantAverageTxnAmount returns the rolling average payment amount for a merchant.
	MerchantAverageTxnAmount(ctx context.Context, merchantID string) (int64, error)
}
