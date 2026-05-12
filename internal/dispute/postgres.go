package dispute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/outbox"
)

// PostgresRepository implements Repository using pgxpool.
type PostgresRepository struct {
	db     *pgxpool.Pool
	outbox *outbox.Writer
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db, outbox: outbox.NewWriter()}
}

// Create inserts a dispute row and emits a dispute.created outbox event in a single transaction.
func (r *PostgresRepository) Create(ctx context.Context, d Dispute) (Dispute, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Dispute{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_disputes.disputes
    (id, merchant_id, payment_id, settlement_id, status, reason, amount, currency,
     evidence, evidence_submitted_at, due_by, resolved_at, notes)
VALUES ($1, $2, $3, NULLIF($4,''), $5, $6, $7, $8,
        $9, $10, $11, $12, NULLIF($13,''))
`,
		d.ID, d.MerchantID, d.PaymentID, d.SettlementID,
		d.Status, d.Reason, d.Amount, d.Currency,
		evidenceJSON(d.Evidence), d.EvidenceSubmittedAt, d.DueBy, d.ResolvedAt, d.Notes,
	); err != nil {
		return Dispute{}, fmt.Errorf("insert dispute: %w", err)
	}

	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "dispute",
		AggregateID:   d.ID,
		EventType:     "dispute.created",
		MerchantID:    d.MerchantID,
		Payload: map[string]any{
			"dispute_id": d.ID,
			"payment_id": d.PaymentID,
			"reason":     d.Reason,
			"amount":     d.Amount,
			"currency":   d.Currency,
		},
	}); err != nil {
		return Dispute{}, fmt.Errorf("write dispute.created outbox event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Dispute{}, err
	}

	return r.GetByID(ctx, d.MerchantID, d.ID)
}

// GetByID returns a dispute by ID scoped to the merchant.
func (r *PostgresRepository) GetByID(ctx context.Context, merchantID, id string) (Dispute, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, merchant_id, payment_id, COALESCE(settlement_id,''), status, reason, amount, currency,
       evidence, evidence_submitted_at, due_by, resolved_at, COALESCE(notes,''),
       created_at, updated_at
FROM paygate_disputes.disputes
WHERE id = $1 AND merchant_id = $2
`, id, merchantID)
	d, err := scanDispute(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Dispute{}, ErrDisputeNotFound
	}
	return d, err
}

// List returns disputes for a merchant ordered by created_at DESC.
func (r *PostgresRepository) List(ctx context.Context, merchantID string, limit int) ([]Dispute, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, payment_id, COALESCE(settlement_id,''), status, reason, amount, currency,
       evidence, evidence_submitted_at, due_by, resolved_at, COALESCE(notes,''),
       created_at, updated_at
FROM paygate_disputes.disputes
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2
`, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var disputes []Dispute
	for rows.Next() {
		d, err := scanDispute(rows)
		if err != nil {
			return nil, err
		}
		disputes = append(disputes, d)
	}
	return disputes, rows.Err()
}

// UpdateStatus transitions the dispute status, sets resolved_at for terminal states,
// and emits a dispute.updated outbox event plus a terminal dispute.<status>
// event when appropriate.
func (r *PostgresRepository) UpdateStatus(ctx context.Context, merchantID, id string, status DisputeState, notes string) (Dispute, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Dispute{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	isTerminal := status == StateWon || status == StateLost || status == StateAccepted
	var resolvedAt *time.Time
	if isTerminal {
		now := time.Now().UTC()
		resolvedAt = &now
	}

	tag, err := tx.Exec(ctx, `
UPDATE paygate_disputes.disputes
SET status = $3,
    notes = CASE WHEN $4 != '' THEN $4 ELSE notes END,
    resolved_at = COALESCE($5, resolved_at),
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, id, merchantID, status, notes, resolvedAt)
	if err != nil {
		return Dispute{}, fmt.Errorf("update dispute status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Dispute{}, ErrDisputeNotFound
	}

	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "dispute",
		AggregateID:   id,
		EventType:     "dispute.updated",
		MerchantID:    merchantID,
		Payload: map[string]any{
			"dispute_id": id,
			"status":     status,
		},
	}); err != nil {
		return Dispute{}, fmt.Errorf("write dispute.updated outbox event: %w", err)
	}

	if isTerminal {
		if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
			AggregateType: "dispute",
			AggregateID:   id,
			EventType:     "dispute." + string(status),
			MerchantID:    merchantID,
			Payload: map[string]any{
				"dispute_id": id,
				"status":     status,
			},
		}); err != nil {
			return Dispute{}, fmt.Errorf("write dispute terminal outbox event: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Dispute{}, err
	}

	return r.GetByID(ctx, merchantID, id)
}

// SubmitEvidence stores evidence on the dispute and stamps evidence_submitted_at.
func (r *PostgresRepository) SubmitEvidence(ctx context.Context, merchantID, id string, evidence map[string]any) (Dispute, error) {
	raw, err := json.Marshal(evidence)
	if err != nil {
		return Dispute{}, fmt.Errorf("marshal evidence: %w", err)
	}

	tag, err := r.db.Exec(ctx, `
UPDATE paygate_disputes.disputes
SET evidence = $3,
    evidence_submitted_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, id, merchantID, raw)
	if err != nil {
		return Dispute{}, fmt.Errorf("update dispute evidence: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Dispute{}, ErrDisputeNotFound
	}

	return r.GetByID(ctx, merchantID, id)
}

func (r *PostgresRepository) GetPaymentReference(ctx context.Context, merchantID, paymentID string) (PaymentReference, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, merchant_id, amount, currency
FROM paygate_payments.payments
WHERE id = $1 AND merchant_id = $2
`, paymentID, merchantID)

	var ref PaymentReference
	if err := row.Scan(&ref.ID, &ref.MerchantID, &ref.Amount, &ref.Currency); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaymentReference{}, ErrPaymentNotFound
		}
		return PaymentReference{}, err
	}
	return ref, nil
}

func (r *PostgresRepository) SettlementExists(ctx context.Context, merchantID, settlementID string) (bool, error) {
	var exists bool
	if err := r.db.QueryRow(ctx, `
SELECT EXISTS(
	SELECT 1
	FROM paygate_settlements.settlements
	WHERE id = $1 AND merchant_id = $2
)
`, settlementID, merchantID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// scanner is the common interface shared by pgx.Row and pgx.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanDispute reads all dispute columns from a row scanner.
func scanDispute(row scanner) (Dispute, error) {
	var d Dispute
	var evidenceRaw []byte

	if err := row.Scan(
		&d.ID, &d.MerchantID, &d.PaymentID, &d.SettlementID,
		&d.Status, &d.Reason, &d.Amount, &d.Currency,
		&evidenceRaw, &d.EvidenceSubmittedAt, &d.DueBy, &d.ResolvedAt, &d.Notes,
		&d.CreatedAt, &d.UpdatedAt,
	); err != nil {
		return Dispute{}, err
	}

	if len(evidenceRaw) > 0 {
		if err := json.Unmarshal(evidenceRaw, &d.Evidence); err != nil {
			return Dispute{}, fmt.Errorf("unmarshal evidence: %w", err)
		}
	}

	return d, nil
}

// evidenceJSON marshals a map to JSON bytes, returning nil if the map is empty.
func evidenceJSON(evidence map[string]any) []byte {
	if len(evidence) == 0 {
		return nil
	}
	raw, _ := json.Marshal(evidence)
	return raw
}
