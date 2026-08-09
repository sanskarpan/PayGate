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
	"github.com/sanskarpan/PayGate/internal/settlement"
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
	var paymentAmount int64
	var paymentFee int64
	var amountRefunded int64
	var settlementID *string
	err = tx.QueryRow(ctx, `
SELECT r.id, r.payment_id, r.order_id, r.merchant_id, r.amount, r.currency, r.reason, r.status,
       p.amount, p.fee, p.amount_refunded, p.settlement_id
	FROM paygate_payments.refunds r
	JOIN paygate_payments.payments p ON p.id = r.payment_id
WHERE r.id = $1
FOR UPDATE
`, refundID).Scan(&ref.ID, &ref.PaymentID, &ref.OrderID, &ref.MerchantID, &ref.Amount, &ref.Currency, &ref.Reason, &ref.Status, &paymentAmount, &paymentFee, &amountRefunded, &settlementID)
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

	prevFeeReversal := settlement.CalculateRefundFeeReversal(paymentAmount, paymentFee, amountRefunded)
	newFeeReversal := settlement.CalculateRefundFeeReversal(paymentAmount, paymentFee, amountRefunded+ref.Amount)
	feeDelta := newFeeReversal - prevFeeReversal
	merchantPayableDelta := ref.Amount - feeDelta

	entries := []ledger.Entry{
		{AccountCode: "REFUND_CLEARING", CreditAmount: ref.Amount, Currency: ref.Currency, Description: "refund clearing credit"},
	}
	if merchantPayableDelta > 0 {
		entries = append(entries, ledger.Entry{AccountCode: "MERCHANT_PAYABLE", DebitAmount: merchantPayableDelta, Currency: ref.Currency, Description: "refund merchant payable reversal"})
	}
	if feeDelta > 0 {
		entries = append(entries, ledger.Entry{AccountCode: "PLATFORM_FEE_REVENUE", DebitAmount: feeDelta, Currency: ref.Currency, Description: "refund fee reversal"})
	}

	if _, err := r.ledger.CreateEntriesTx(ctx, tx, ref.MerchantID, "refund", refundID, "refund processed", entries); err != nil {
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
	if settlementID != nil && *settlementID != "" {
		if _, err := tx.Exec(ctx, `
INSERT INTO paygate_settlements.settlement_adjustments
    (id, merchant_id, settlement_id, payment_id, refund_id, adjustment_type, amount, currency, reason)
VALUES ($1, $2, $3, $4, $5, 'refund_processed', $6, $7, $8)
`, idgen.New("sadj"), ref.MerchantID, *settlementID, ref.PaymentID, refundID, merchantPayableDelta, ref.Currency, ref.Reason); err != nil {
			return Refund{}, err
		}
		if _, err := tx.Exec(ctx, `
UPDATE paygate_settlements.settlements
SET rollback_marked_at = COALESCE(rollback_marked_at, $3),
    rollback_reason = CASE WHEN rollback_reason IS NULL OR rollback_reason = '' THEN $4 ELSE rollback_reason END,
    on_hold = TRUE,
    hold_reason = COALESCE(NULLIF(hold_reason, ''), $4),
    held_at = COALESCE(held_at, $3),
    updated_at = $3
WHERE id = $1 AND merchant_id = $2
`, *settlementID, ref.MerchantID, now, "post-settlement refund adjustment"); err != nil {
			return Refund{}, err
		}
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

func (r *PostgresRepository) ReverseRefund(ctx context.Context, merchantID, refundID, reason string) (Refund, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Refund{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var ref Refund
	var paymentAmount int64
	var paymentFee int64
	var amountRefunded int64
	var amountRefundedPending int64
	var settlementID *string
	err = tx.QueryRow(ctx, `
SELECT r.id, r.payment_id, r.order_id, r.merchant_id, r.amount, r.currency, r.reason, r.status,
       p.amount, p.fee, p.amount_refunded, p.amount_refunded_pending, p.settlement_id
FROM paygate_payments.refunds r
JOIN paygate_payments.payments p ON p.id = r.payment_id
WHERE r.id = $1 AND r.merchant_id = $2
FOR UPDATE
`, refundID, merchantID).Scan(&ref.ID, &ref.PaymentID, &ref.OrderID, &ref.MerchantID, &ref.Amount, &ref.Currency, &ref.Reason, &ref.Status, &paymentAmount, &paymentFee, &amountRefunded, &amountRefundedPending, &settlementID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Refund{}, ErrRefundNotFound
		}
		return Refund{}, err
	}
	if _, err := Transition(ref.Status, EventReverse); err != nil {
		return Refund{}, err
	}
	if amountRefunded < ref.Amount {
		return Refund{}, ErrInvalidTransition
	}

	prevFeeReversal := settlement.CalculateRefundFeeReversal(paymentAmount, paymentFee, amountRefunded)
	newAmountRefunded := amountRefunded - ref.Amount
	newFeeReversal := settlement.CalculateRefundFeeReversal(paymentAmount, paymentFee, newAmountRefunded)
	feeDelta := prevFeeReversal - newFeeReversal
	merchantPayableDelta := ref.Amount - feeDelta
	now := time.Now().UTC()

	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.refunds
SET status = 'reversed',
    reversal_reason = NULLIF($2, ''),
    reversed_at = $3,
    updated_at = $3
WHERE id = $1
`, refundID, reason, now); err != nil {
		return Refund{}, err
	}

	refundStatus := "partial"
	switch total := newAmountRefunded + amountRefundedPending; {
	case total <= 0:
		refundStatus = "none"
	case total >= paymentAmount:
		refundStatus = "full"
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET amount_refunded = amount_refunded - $1,
    refund_status = $3,
    updated_at = NOW()
WHERE id = $2
`, ref.Amount, ref.PaymentID, refundStatus); err != nil {
		return Refund{}, err
	}

	entries := []ledger.Entry{
		{AccountCode: "REFUND_CLEARING", DebitAmount: ref.Amount, Currency: ref.Currency, Description: "refund reversal clearing debit"},
	}
	if merchantPayableDelta > 0 {
		entries = append(entries, ledger.Entry{AccountCode: "MERCHANT_PAYABLE", CreditAmount: merchantPayableDelta, Currency: ref.Currency, Description: "refund reversal merchant payable restore"})
	}
	if feeDelta > 0 {
		entries = append(entries, ledger.Entry{AccountCode: "PLATFORM_FEE_REVENUE", CreditAmount: feeDelta, Currency: ref.Currency, Description: "refund reversal fee restore"})
	}
	if _, err := r.ledger.CreateEntriesTx(ctx, tx, ref.MerchantID, "refund_reversal", refundID, "refund reversed", entries); err != nil {
		return Refund{}, err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "refund",
		AggregateID:   refundID,
		EventType:     "refund.reversed",
		MerchantID:    ref.MerchantID,
		Payload: map[string]any{
			"refund_id":  refundID,
			"payment_id": ref.PaymentID,
			"amount":     ref.Amount,
			"reason":     reason,
		},
	}); err != nil {
		return Refund{}, err
	}
	if settlementID != nil && *settlementID != "" {
		if _, err := tx.Exec(ctx, `
INSERT INTO paygate_settlements.settlement_adjustments
    (id, merchant_id, settlement_id, payment_id, refund_id, adjustment_type, amount, currency, reason)
VALUES ($1, $2, $3, $4, $5, 'refund_reversed', $6, $7, $8)
`, idgen.New("sadj"), ref.MerchantID, *settlementID, ref.PaymentID, refundID, merchantPayableDelta, ref.Currency, reason); err != nil {
			return Refund{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Refund{}, err
	}

	ref.Status = StateReversed
	ref.ReversedAt = &now
	ref.ReversalReason = reason
	return ref, nil
}

// GetRefund fetches a single refund by ID and merchantID.
func (r *PostgresRepository) GetRefund(ctx context.Context, merchantID, refundID string) (Refund, error) {
	var ref Refund
	var notesRaw []byte
	var gatewayRefundID *string
	var reversalReason *string
	err := r.db.QueryRow(ctx, `
SELECT id, payment_id, order_id, merchant_id, amount, currency,
       reason, status, gateway_refund_id, notes, processed_at, reversal_reason, reversed_at, created_at, updated_at
FROM paygate_payments.refunds
WHERE id = $1 AND merchant_id = $2
`, refundID, merchantID).Scan(
		&ref.ID, &ref.PaymentID, &ref.OrderID, &ref.MerchantID, &ref.Amount, &ref.Currency,
		&ref.Reason, &ref.Status, &gatewayRefundID, &notesRaw, &ref.ProcessedAt, &reversalReason, &ref.ReversedAt,
		&ref.CreatedAt, &ref.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Refund{}, ErrRefundNotFound
		}
		return Refund{}, err
	}
	if gatewayRefundID != nil {
		ref.GatewayRefundID = *gatewayRefundID
	}
	if reversalReason != nil {
		ref.ReversalReason = *reversalReason
	}
	if len(notesRaw) > 0 {
		_ = json.Unmarshal(notesRaw, &ref.Notes)
	}
	return ref, nil
}

// ListRefunds returns all refunds for a payment, scoped to merchantID.
func (r *PostgresRepository) PaymentExists(ctx context.Context, merchantID, paymentID string) (bool, error) {
	if merchantID == "" || paymentID == "" {
		return false, nil
	}
	var exists bool
	err := r.db.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM paygate_payments.payments WHERE id = $1 AND merchant_id = $2
)
`, paymentID, merchantID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *PostgresRepository) ListRefunds(ctx context.Context, merchantID, paymentID string) ([]Refund, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, payment_id, order_id, merchant_id, amount, currency,
       reason, status, gateway_refund_id, notes, processed_at, reversal_reason, reversed_at, created_at, updated_at
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
		var gatewayRefundID *string
		var reversalReason *string
		if err := rows.Scan(
			&ref.ID, &ref.PaymentID, &ref.OrderID, &ref.MerchantID, &ref.Amount, &ref.Currency,
			&ref.Reason, &ref.Status, &gatewayRefundID, &notesRaw, &ref.ProcessedAt, &reversalReason, &ref.ReversedAt,
			&ref.CreatedAt, &ref.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if gatewayRefundID != nil {
			ref.GatewayRefundID = *gatewayRefundID
		}
		if reversalReason != nil {
			ref.ReversalReason = *reversalReason
		}
		if len(notesRaw) > 0 {
			_ = json.Unmarshal(notesRaw, &ref.Notes)
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}
