package tokenization

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateCardToken(ctx context.Context, in CreateCardTokenRecordInput) (CardToken, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CardToken{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var out CardToken
	if err := tx.QueryRow(ctx, `
INSERT INTO paygate_vault.card_tokens
(id, merchant_id, token_class, status, fingerprint_hash, last4, bin, brand, exp_month, exp_year, customer_ref, network_reference)
VALUES ($1,$2,$3,'active',$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING id, merchant_id, token_class, status, last4, bin, brand, exp_month, exp_year, customer_ref, network_reference, created_at, last_used_at, consumed_at, disabled_at
`, in.ID, in.MerchantID, in.TokenClass, in.FingerprintHash, in.Last4, in.BIN, in.Brand, in.ExpMonth, in.ExpYear, in.CustomerRef, in.NetworkReference).Scan(
		&out.ID,
		&out.MerchantID,
		&out.TokenClass,
		&out.Status,
		&out.Last4,
		&out.BIN,
		&out.Brand,
		&out.ExpMonth,
		&out.ExpYear,
		&out.CustomerRef,
		&out.NetworkReference,
		&out.CreatedAt,
		&out.LastUsedAt,
		&out.ConsumedAt,
		&out.DisabledAt,
	); err != nil {
		return CardToken{}, fmt.Errorf("insert card token: %w", err)
	}
	if err := writeAuditTx(ctx, tx, out.ID, out.MerchantID, "", "tokenized", "", "merchant_api"); err != nil {
		return CardToken{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CardToken{}, err
	}
	return out, nil
}

func (r *PostgresRepository) GetCardToken(ctx context.Context, merchantID, tokenID, action, reason, actor string) (CardToken, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CardToken{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out, err := getCardTokenTx(ctx, tx, merchantID, tokenID, false)
	if err != nil {
		return CardToken{}, err
	}
	if err := writeAuditTx(ctx, tx, out.ID, out.MerchantID, "", action, reason, actor); err != nil {
		return CardToken{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CardToken{}, err
	}
	return out, nil
}

func (r *PostgresRepository) PrepareCardTokenAuthorization(ctx context.Context, merchantID, tokenID, paymentID string, usedAt time.Time) (CardToken, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CardToken{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out, err := getCardTokenTx(ctx, tx, merchantID, tokenID, true)
	if err != nil {
		return CardToken{}, err
	}
	if out.Status != CardTokenStatusActive {
		return CardToken{}, ErrCardTokenInactive
	}
	if out.TokenClass == CardTokenClassSingleUse {
		if _, err := tx.Exec(ctx, `
UPDATE paygate_vault.card_tokens
SET status = 'reserved', last_used_at = $2, reserved_at = $2, reserved_payment_id = $3
WHERE id = $1
`, tokenID, usedAt, paymentID); err != nil {
			return CardToken{}, fmt.Errorf("consume card token: %w", err)
		}
		out.Status = CardTokenStatusReserved
		out.LastUsedAt = &usedAt
	} else {
		if _, err := tx.Exec(ctx, `
UPDATE paygate_vault.card_tokens
SET last_used_at = $2
WHERE id = $1
`, tokenID, usedAt); err != nil {
			return CardToken{}, fmt.Errorf("touch reusable token: %w", err)
		}
		out.LastUsedAt = &usedAt
	}
	if err := writeAuditTx(ctx, tx, out.ID, out.MerchantID, paymentID, "authorize", "", "payment_service"); err != nil {
		return CardToken{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CardToken{}, err
	}
	return out, nil
}

func (r *PostgresRepository) CompleteCardTokenAuthorization(ctx context.Context, merchantID, tokenID, paymentID string, success bool, reason string, usedAt time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out, err := getCardTokenTx(ctx, tx, merchantID, tokenID, true)
	if err != nil {
		return err
	}
	if out.TokenClass == CardTokenClassSingleUse {
		if success {
			_, err = tx.Exec(ctx, `
UPDATE paygate_vault.card_tokens
SET status = 'consumed', consumed_at = $2, reserved_at = NULL, reserved_payment_id = ''
WHERE id = $1
`, tokenID, usedAt)
		} else {
			_, err = tx.Exec(ctx, `
UPDATE paygate_vault.card_tokens
SET status = 'active', reserved_at = NULL, reserved_payment_id = ''
WHERE id = $1
`, tokenID)
		}
		if err != nil {
			return fmt.Errorf("finalize card token authorization: %w", err)
		}
	}
	action := "authorize_complete"
	if !success {
		action = "authorize_release"
	}
	if err := writeAuditTx(ctx, tx, out.ID, out.MerchantID, paymentID, action, reason, "payment_service"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) DisableCardToken(ctx context.Context, merchantID, tokenID, reason, actor string, disabledAt time.Time) (CardToken, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CardToken{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out, err := getCardTokenTx(ctx, tx, merchantID, tokenID, true)
	if err != nil {
		return CardToken{}, err
	}
	if out.Status != CardTokenStatusDisabled {
		if _, err := tx.Exec(ctx, `
UPDATE paygate_vault.card_tokens
SET status = 'disabled', disabled_at = $2
WHERE id = $1
`, tokenID, disabledAt); err != nil {
			return CardToken{}, fmt.Errorf("disable card token: %w", err)
		}
		out.Status = CardTokenStatusDisabled
		out.DisabledAt = &disabledAt
	}
	if err := writeAuditTx(ctx, tx, out.ID, out.MerchantID, "", "disable", reason, actor); err != nil {
		return CardToken{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CardToken{}, err
	}
	return out, nil
}

func getCardTokenTx(ctx context.Context, tx pgx.Tx, merchantID, tokenID string, lock bool) (CardToken, error) {
	query := `
SELECT id, merchant_id, token_class, status, last4, bin, brand, exp_month, exp_year, customer_ref, network_reference, created_at, last_used_at, consumed_at, disabled_at
FROM paygate_vault.card_tokens
WHERE id = $1 AND merchant_id = $2`
	if lock {
		query += ` FOR UPDATE`
	}
	var out CardToken
	if err := tx.QueryRow(ctx, query, tokenID, merchantID).Scan(
		&out.ID,
		&out.MerchantID,
		&out.TokenClass,
		&out.Status,
		&out.Last4,
		&out.BIN,
		&out.Brand,
		&out.ExpMonth,
		&out.ExpYear,
		&out.CustomerRef,
		&out.NetworkReference,
		&out.CreatedAt,
		&out.LastUsedAt,
		&out.ConsumedAt,
		&out.DisabledAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CardToken{}, ErrCardTokenNotFound
		}
		return CardToken{}, err
	}
	return out, nil
}

func writeAuditTx(ctx context.Context, tx pgx.Tx, tokenID, merchantID, paymentID, action, reason, actor string) error {
	_, err := tx.Exec(ctx, `
INSERT INTO paygate_vault.card_token_access_audit
(id, token_id, merchant_id, payment_id, action, reason, actor)
VALUES ($1,$2,$3,$4,$5,$6,$7)
`, idgen.New("ctaudit"), tokenID, merchantID, paymentID, action, reason, actor)
	return err
}
