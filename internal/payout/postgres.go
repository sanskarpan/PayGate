package payout

import (
	"context"
	"errors"
	"fmt"
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

// CreateForSettlement inserts a payout in pending state for the given settlement
// and writes a payout.created outbox event.
func (r *PostgresRepository) CreateForSettlement(ctx context.Context, merchantID, settlementID string, amount int64, currency string) (Payout, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Payout{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id := idgen.New("payout")
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_payouts.payouts
    (id, merchant_id, settlement_id, status, amount, currency)
VALUES ($1, $2, $3, 'pending', $4, $5)
`, id, merchantID, settlementID, amount, currency); err != nil {
		return Payout{}, fmt.Errorf("insert payout: %w", err)
	}

	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "payout",
		AggregateID:   id,
		EventType:     "payout.created",
		MerchantID:    merchantID,
		Payload: map[string]any{
			"payout_id":     id,
			"settlement_id": settlementID,
			"amount":        amount,
			"currency":      currency,
		},
	}); err != nil {
		return Payout{}, fmt.Errorf("write payout.created outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Payout{}, err
	}
	return r.GetByID(ctx, merchantID, id)
}

// GetByID returns a payout by ID scoped to the merchant.
func (r *PostgresRepository) GetByID(ctx context.Context, merchantID, id string) (Payout, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, merchant_id, settlement_id, status, amount, currency,
       bank_reference, failure_reason, initiated_at, completed_at, created_at, updated_at
FROM paygate_payouts.payouts
WHERE id = $1 AND merchant_id = $2
`, id, merchantID)
	p, err := scanPayout(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payout{}, ErrPayoutNotFound
	}
	return p, err
}

// GetBySettlementID returns the payout for a settlement scoped to the merchant.
func (r *PostgresRepository) GetBySettlementID(ctx context.Context, merchantID, settlementID string) (Payout, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, merchant_id, settlement_id, status, amount, currency,
       bank_reference, failure_reason, initiated_at, completed_at, created_at, updated_at
FROM paygate_payouts.payouts
WHERE settlement_id = $1 AND merchant_id = $2
LIMIT 1
`, settlementID, merchantID)
	p, err := scanPayout(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payout{}, ErrPayoutNotFound
	}
	return p, err
}

// List returns the most recent payouts for a merchant up to limit.
func (r *PostgresRepository) List(ctx context.Context, merchantID string, limit int) ([]Payout, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, settlement_id, status, amount, currency,
       bank_reference, failure_reason, initiated_at, completed_at, created_at, updated_at
FROM paygate_payouts.payouts
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2
`, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Payout
	for rows.Next() {
		p, err := scanPayout(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Initiate transitions a payout from pending to processing and writes a payout.initiated outbox event.
func (r *PostgresRepository) Initiate(ctx context.Context, merchantID, id string) (Payout, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Payout{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current PayoutState
	if err := tx.QueryRow(ctx, `SELECT status FROM paygate_payouts.payouts WHERE id=$1 AND merchant_id=$2 FOR UPDATE`, id, merchantID).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Payout{}, ErrPayoutNotFound
		}
		return Payout{}, err
	}
	if _, err := Transition(current, EventInitiate); err != nil {
		return Payout{}, err
	}

	if _, err := tx.Exec(ctx, `
UPDATE paygate_payouts.payouts
SET status = 'processing', initiated_at = NOW(), updated_at = NOW()
WHERE id = $1
`, id); err != nil {
		return Payout{}, fmt.Errorf("update payout initiate: %w", err)
	}

	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "payout",
		AggregateID:   id,
		EventType:     "payout.initiated",
		MerchantID:    merchantID,
		Payload:       map[string]any{"payout_id": id},
	}); err != nil {
		return Payout{}, fmt.Errorf("write payout.initiated outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Payout{}, err
	}
	return r.GetByID(ctx, merchantID, id)
}

// Complete transitions a payout from processing to completed, records the bank reference,
// writes a ledger entry (Dr. SETTLEMENT_CLEARING / Cr. MERCHANT_BANK_PAYOUT), and writes
// a payout.completed outbox event.
func (r *PostgresRepository) Complete(ctx context.Context, merchantID, id, bankReference string) (Payout, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Payout{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current PayoutState
	var amount int64
	var currency string
	if err := tx.QueryRow(ctx, `
SELECT status, amount, currency FROM paygate_payouts.payouts
WHERE id=$1 AND merchant_id=$2 FOR UPDATE
`, id, merchantID).Scan(&current, &amount, &currency); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Payout{}, ErrPayoutNotFound
		}
		return Payout{}, err
	}
	if _, err := Transition(current, EventComplete); err != nil {
		return Payout{}, err
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payouts.payouts
SET status = 'completed', bank_reference = $2, completed_at = $3, updated_at = $3
WHERE id = $1
`, id, bankReference, now); err != nil {
		return Payout{}, fmt.Errorf("update payout complete: %w", err)
	}

	// Ledger: Dr. SETTLEMENT_CLEARING / Cr. MERCHANT_BANK_PAYOUT
	if _, err := r.ledger.CreateEntriesTx(ctx, tx, merchantID, "payout", id,
		fmt.Sprintf("payout %s completed", id),
		[]ledger.Entry{
			{AccountCode: "SETTLEMENT_CLEARING", DebitAmount: amount, Currency: currency, Description: "settlement clearing debit on payout"},
			{AccountCode: "MERCHANT_BANK_PAYOUT", CreditAmount: amount, Currency: currency, Description: "merchant bank payout credit"},
		},
	); err != nil {
		return Payout{}, fmt.Errorf("write payout ledger entries: %w", err)
	}

	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "payout",
		AggregateID:   id,
		EventType:     "payout.completed",
		MerchantID:    merchantID,
		Payload: map[string]any{
			"payout_id":      id,
			"bank_reference": bankReference,
			"amount":         amount,
			"currency":       currency,
		},
	}); err != nil {
		return Payout{}, fmt.Errorf("write payout.completed outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Payout{}, err
	}
	return r.GetByID(ctx, merchantID, id)
}

// Fail transitions a payout from processing to failed and writes a payout.failed outbox event.
func (r *PostgresRepository) Fail(ctx context.Context, merchantID, id, reason string) (Payout, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Payout{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current PayoutState
	if err := tx.QueryRow(ctx, `SELECT status FROM paygate_payouts.payouts WHERE id=$1 AND merchant_id=$2 FOR UPDATE`, id, merchantID).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Payout{}, ErrPayoutNotFound
		}
		return Payout{}, err
	}
	if _, err := Transition(current, EventFail); err != nil {
		return Payout{}, err
	}

	if _, err := tx.Exec(ctx, `
UPDATE paygate_payouts.payouts
SET status = 'failed', failure_reason = $2, updated_at = NOW()
WHERE id = $1
`, id, reason); err != nil {
		return Payout{}, fmt.Errorf("update payout fail: %w", err)
	}

	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "payout",
		AggregateID:   id,
		EventType:     "payout.failed",
		MerchantID:    merchantID,
		Payload:       map[string]any{"payout_id": id, "reason": reason},
	}); err != nil {
		return Payout{}, fmt.Errorf("write payout.failed outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Payout{}, err
	}
	return r.GetByID(ctx, merchantID, id)
}

// scanPayout is a helper that scans a payout row from any pgx row-like interface.
type scannable interface {
	Scan(dest ...any) error
}

func scanPayout(row scannable) (Payout, error) {
	var p Payout
	var bankRef *string
	var failureReason *string
	err := row.Scan(
		&p.ID, &p.MerchantID, &p.SettlementID, &p.Status,
		&p.Amount, &p.Currency,
		&bankRef, &failureReason,
		&p.InitiatedAt, &p.CompletedAt,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Payout{}, err
	}
	if bankRef != nil {
		p.BankReference = *bankRef
	}
	if failureReason != nil {
		p.FailureReason = *failureReason
	}
	return p, nil
}
