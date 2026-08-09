package refund

import "context"

// Repository is the persistence interface for the refund domain.
// The Postgres implementation lives in postgres.go.
type Repository interface {
	// CreateRefund opens a transaction, validates eligibility (SELECT FOR UPDATE on
	// the payment row), reserves amount_refunded_pending, inserts the refund record
	// as 'created', writes an outbox event, and returns the new Refund.
	CreateRefund(ctx context.Context, in CreateInput) (Refund, error)

	// ProcessRefund transitions a refund from processing → processed, writes ledger
	// reversal entries, updates payment.amount_refunded / clears pending, and writes
	// outbox event — all in one transaction.
	ProcessRefund(ctx context.Context, refundID string) (Refund, error)

	// FailRefund transitions a refund from processing → failed, releases the
	// amount_refunded_pending reservation, and writes an outbox event.
	FailRefund(ctx context.Context, refundID string) (Refund, error)

	// ReverseRefund transitions a processed refund to reversed and writes
	// corrective ledger entries so the money path remains auditable and forward-only.
	ReverseRefund(ctx context.Context, merchantID, refundID, reason string) (Refund, error)

	// GetRefund fetches a refund by ID, scoped to merchantID.
	GetRefund(ctx context.Context, merchantID, refundID string) (Refund, error)

	// ListRefunds returns all refunds for a payment, scoped to merchantID.
	ListRefunds(ctx context.Context, merchantID, paymentID string) ([]Refund, error)

	// PaymentExists reports whether the payment exists and belongs to merchantID,
	// so listing refunds for an unknown payment can 404 instead of returning an
	// empty collection.
	PaymentExists(ctx context.Context, merchantID, paymentID string) (bool, error)
}

// CreateInput carries the caller-supplied fields for a new refund.
type CreateInput struct {
	PaymentID  string
	MerchantID string
	Amount     int64
	Reason     string
	Notes      map[string]any
}
