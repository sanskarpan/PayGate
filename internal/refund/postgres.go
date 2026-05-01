package refund

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/outbox"
)

// PostgresRepository implements Repository using pgxpool.
type PostgresRepository struct {
	db     *pgxpool.Pool
	ledger *ledger.Service
	outbox *outbox.Writer
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(db *pgxpool.Pool, ledgerSvc *ledger.Service) *PostgresRepository {
	return &PostgresRepository{db: db, ledger: ledgerSvc, outbox: outbox.NewWriter()}
}

// CreateRefund validates eligibility, reserves amount_refunded_pending,
// inserts the refund as 'created', immediately initiates processing (sets
// status to 'processing'), and writes an outbox event — all in one transaction.
func (r *PostgresRepository) CreateRefund(ctx context.Context, in CreateInput) (Refund, error) {
	if in.Amount <= 0 {
		return Refund{}, ErrZeroRefundAmount
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Refund{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the payment row to prevent concurrent over-refunding.
	var snap PaymentSnapshot
	err = tx.QueryRow(ctx, `
SELECT id, order_id, merchant_id, amount, currency, status,
       amount_refunded, amount_refunded_pending
FROM paygate_payments.payments
WHERE id = $1 AND merchant_id = $2
FOR UPDATE
`, in.PaymentID, in.MerchantID).Scan(
		&snap.ID, &snap.OrderID, &snap.MerchantID,
		&snap.Amount, &snap.Currency, &snap.Status,
		&snap.AmountRefunded, &snap.AmountRefundedPending,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Refund{}, ErrRefundNotFound
		}
		return Refund{}, err
	}

	if snap.Status != "captured" {
		return Refund{}, ErrPaymentNotCaptured
	}
	if in.Amount > snap.RefundableBalance() {
		return Refund{}, ErrRefundAmountExceedsRefundable
	}

	refundID := idgen.New("rfnd")
	notesJSON, _ := json.Marshal(in.Notes)

	// Insert as 'processing' directly (simulator resolves synchronously).
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_payments.refunds
(id, payment_id, order_id, merchant_id, amount, currency, reason, status, notes)
VALUES ($1,$2,$3,$4,$5,$6,$7,'processing',$8)
`, refundID, in.PaymentID, snap.OrderID, in.MerchantID, in.Amount, snap.Currency, in.Reason, notesJSON); err != nil {
		return Refund{}, err
	}

	// Reserve the pending refund amount on the payment.
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET amount_refunded_pending = amount_refunded_pending + $1,
    refund_status = CASE
        WHEN amount_refunded + $1 >= amount THEN 'full'
        ELSE 'partial'
    END,
    updated_at = NOW()
WHERE id = $2
`, in.Amount, in.PaymentID); err != nil {
		return Refund{}, err
	}

	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "refund",
		AggregateID:   refundID,
		EventType:     "refund.created",
		MerchantID:    in.MerchantID,
		Payload: map[string]any{
			"refund_id":  refundID,
			"payment_id": in.PaymentID,
			"amount":     in.Amount,
			"currency":   snap.Currency,
		},
	}); err != nil {
		return Refund{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Refund{}, err
	}

	return r.GetRefund(ctx, in.MerchantID, refundID)
}

// ProcessRefund transitions processing → processed, writes ledger reversal
// entries (Dr. MERCHANT_PAYABLE / Cr. REFUND_CLEARING), moves amount from
// pending to confirmed, and writes an outbox event.
func (r *PostgresRepository) ProcessRefund(ctx context.Context, refundID string) (Refund, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Refund{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var ref Refund
	var notesRaw []byte
	err = tx.QueryRow(ctx, `
SELECT id, payment_id, order_id, merchant_id, amount, currency, reason, status
FROM paygate_payments.refunds
WHERE id = $1
FOR UPDATE
`, refundID).Scan(&ref.ID, &ref.PaymentID, &ref.OrderID, &ref.MerchantID, &ref.Amount, &ref.Currency, &ref.Reason, &ref.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Refund{}, ErrRefundNotFound
		}
		return Refund{}, err
	}
	_ = notesRaw

	if _, err := Transition(ref.Status, EventSuccess); err != nil {
		return Refund{}, err
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.refunds
SET status = 'processed', processed_at = $2, updated_at = $2
WHERE id = $1
`, refundID, now); err != nil {
		return Refund{}, err
	}

	// Move amount from pending to confirmed on the payment.
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET amount_refunded         = amount_refunded + $1,
    amount_refunded_pending = amount_refunded_pending - $1,
    updated_at = NOW()
WHERE id = $2
`, ref.Amount, ref.PaymentID); err != nil {
		return Refund{}, err
	}

	// Ledger reversal: Dr. MERCHANT_PAYABLE / Cr. REFUND_CLEARING
	if _, err := r.ledger.CreateEntriesTx(ctx, tx, ref.MerchantID, "refund", refundID, "refund processed", []ledger.Entry{
		{AccountCode: "MERCHANT_PAYABLE", DebitAmount: ref.Amount, Currency: ref.Currency, Description: "refund debit"},
		{AccountCode: "REFUND_CLEARING", CreditAmount: ref.Amount, Currency: ref.Currency, Description: "refund clearing credit"},
	}); err != nil {
		return Refund{}, err
	}

	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "refund",
		AggregateID:   refundID,
		EventType:     "refund.processed",
		MerchantID:    ref.MerchantID,
		Payload:       map[string]any{"refund_id": refundID, "payment_id": ref.PaymentID, "amount": ref.Amount},
	}); err != nil {
		return Refund{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Refund{}, err
	}

	ref.Status = StateProcessed
	ref.ProcessedAt = &now
	return ref, nil
}

// FailRefund transitions processing → failed and releases amount_refunded_pending.
func (r *PostgresRepository) FailRefund(ctx context.Context, refundID string) (Refund, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Refund{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var ref Refund
	err = tx.QueryRow(ctx, `
SELECT id, payment_id, merchant_id, amount, currency, status
FROM paygate_payments.refunds WHERE id = $1 FOR UPDATE
`, refundID).Scan(&ref.ID, &ref.PaymentID, &ref.MerchantID, &ref.Amount, &ref.Currency, &ref.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Refund{}, ErrRefundNotFound
		}
		return Refund{}, err
	}

	if _, err := Transition(ref.Status, EventFailure); err != nil {
		return Refund{}, err
	}

	if _, err := tx.Exec(ctx, `UPDATE paygate_payments.refunds SET status='failed', updated_at=NOW() WHERE id=$1`, refundID); err != nil {
		return Refund{}, err
	}

	// Release the pending reservation.
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET amount_refunded_pending = amount_refunded_pending - $1, updated_at = NOW()
WHERE id = $2
`, ref.Amount, ref.PaymentID); err != nil {
		return Refund{}, err
	}

	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "refund",
		AggregateID:   refundID,
		EventType:     "refund.failed",
		MerchantID:    ref.MerchantID,
		Payload:       map[string]any{"refund_id": refundID, "payment_id": ref.PaymentID},
	}); err != nil {
		return Refund{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Refund{}, err
	}
	ref.Status = StateFailed
	return ref, nil
}

// GetRefund fetches a single refund by ID and merchantID.
func (r *PostgresRepository) GetRefund(ctx context.Context, merchantID, refundID string) (Refund, error) {
	var ref Refund
	var notesRaw []byte
	err := r.db.QueryRow(ctx, `
SELECT id, payment_id, order_id, merchant_id, amount, currency,
       reason, status, gateway_refund_id, notes, processed_at, created_at, updated_at
FROM paygate_payments.refunds
WHERE id = $1 AND merchant_id = $2
`, refundID, merchantID).Scan(
		&ref.ID, &ref.PaymentID, &ref.OrderID, &ref.MerchantID, &ref.Amount, &ref.Currency,
		&ref.Reason, &ref.Status, &ref.GatewayRefundID, &notesRaw, &ref.ProcessedAt,
		&ref.CreatedAt, &ref.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Refund{}, ErrRefundNotFound
		}
		return Refund{}, err
	}
	if len(notesRaw) > 0 {
		_ = json.Unmarshal(notesRaw, &ref.Notes)
	}
	return ref, nil
}

// ListRefunds returns all refunds for a payment, scoped to merchantID.
func (r *PostgresRepository) ListRefunds(ctx context.Context, merchantID, paymentID string) ([]Refund, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, payment_id, order_id, merchant_id, amount, currency,
       reason, status, gateway_refund_id, notes, processed_at, created_at, updated_at
FROM paygate_payments.refunds
WHERE merchant_id = $1 AND payment_id = $2
ORDER BY created_at DESC
`, merchantID, paymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Refund
	for rows.Next() {
		var ref Refund
		var notesRaw []byte
		if err := rows.Scan(
			&ref.ID, &ref.PaymentID, &ref.OrderID, &ref.MerchantID, &ref.Amount, &ref.Currency,
			&ref.Reason, &ref.Status, &ref.GatewayRefundID, &notesRaw, &ref.ProcessedAt,
			&ref.CreatedAt, &ref.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if len(notesRaw) > 0 {
			_ = json.Unmarshal(notesRaw, &ref.Notes)
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}
