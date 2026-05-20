package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.db.Begin(ctx)
}

func (r *Repository) InsertTransactionWithEntriesTx(ctx context.Context, tx pgx.Tx, txnID, merchantID, sourceType, sourceID, description string, entries []Entry) error {
	var totalDebit, totalCredit int64
	// Derive currency from first entry (all entries in one transaction must share currency).
	txnCurrency := "INR"
	for _, e := range entries {
		totalDebit += e.DebitAmount
		totalCredit += e.CreditAmount
		if e.Currency != "" {
			txnCurrency = e.Currency
		}
	}

	_, err := tx.Exec(ctx, `
INSERT INTO paygate_ledger.ledger_transactions
(id, merchant_id, source_type, source_id, currency, total_debit, total_credit, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`, txnID, merchantID, sourceType, sourceID, txnCurrency, totalDebit, totalCredit, description)
	if err != nil {
		return fmt.Errorf("insert ledger transaction: %w", err)
	}

	for _, e := range entries {
		entryCurrency := e.Currency
		if entryCurrency == "" {
			entryCurrency = txnCurrency
		}
		_, err := tx.Exec(ctx, `
INSERT INTO paygate_ledger.ledger_entries
(id, transaction_id, merchant_id, account_code, debit_amount, credit_amount, currency, source_type, source_id, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
`, idgen.New("le"), txnID, merchantID, e.AccountCode, e.DebitAmount, e.CreditAmount, entryCurrency, sourceType, sourceID, e.Description)
		if err != nil {
			return fmt.Errorf("insert ledger entry: %w", err)
		}
	}
	return nil
}

func (r *Repository) GetBalance(ctx context.Context, merchantID, accountCode string) (int64, error) {
	var balance int64
	err := r.db.QueryRow(ctx, `
SELECT COALESCE(SUM(debit_amount - credit_amount), 0)
FROM paygate_ledger.ledger_entries
WHERE merchant_id = $1 AND account_code = $2
`, merchantID, accountCode).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("query ledger balance: %w", err)
	}
	return balance, nil
}

func (r *Repository) GetBalanceByCurrency(ctx context.Context, merchantID, accountCode, currency string) (int64, error) {
	var balance int64
	err := r.db.QueryRow(ctx, `
SELECT COALESCE(SUM(debit_amount - credit_amount), 0)
FROM paygate_ledger.ledger_entries
WHERE merchant_id = $1 AND account_code = $2 AND currency = $3
`, merchantID, accountCode, currency).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("query ledger balance by currency: %w", err)
	}
	return balance, nil
}

func (r *Repository) CreateHold(ctx context.Context, in CreateHoldInput) (Hold, error) {
	id := idgen.New("hold")
	if in.Currency == "" {
		in.Currency = "INR"
	}
	row := r.db.QueryRow(ctx, `
INSERT INTO paygate_ledger.ledger_holds
    (id, merchant_id, account_code, source_type, source_id, reason, currency, amount, status, idempotency_key, expires_at)
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8, 'active', $9, $10)
RETURNING id, merchant_id, account_code, source_type, source_id, reason, currency, amount, status,
          idempotency_key, target_account_code, description, expires_at, released_at, committed_at, created_at, updated_at
`, id, in.MerchantID, in.AccountCode, in.SourceType, in.SourceID, in.Reason, in.Currency, in.Amount, in.IdempotencyKey, in.ExpiresAt)
	hold, err := scanHold(row)
	if err != nil {
		return Hold{}, fmt.Errorf("insert ledger hold: %w", err)
	}
	return hold, nil
}

func (r *Repository) GetHold(ctx context.Context, merchantID, holdID string) (Hold, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, merchant_id, account_code, source_type, source_id, reason, currency, amount, status,
       idempotency_key, target_account_code, description, expires_at, released_at, committed_at, created_at, updated_at
FROM paygate_ledger.ledger_holds
WHERE id = $1 AND merchant_id = $2
`, holdID, merchantID)
	hold, err := scanHold(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Hold{}, ErrHoldNotFound
	}
	if err != nil {
		return Hold{}, err
	}
	return hold, nil
}

func (r *Repository) ListHolds(ctx context.Context, merchantID string, limit int) ([]Hold, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, account_code, source_type, source_id, reason, currency, amount, status,
       idempotency_key, target_account_code, description, expires_at, released_at, committed_at, created_at, updated_at
FROM paygate_ledger.ledger_holds
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2
`, merchantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list ledger holds: %w", err)
	}
	defer rows.Close()

	var holds []Hold
	for rows.Next() {
		hold, err := scanHold(rows)
		if err != nil {
			return nil, err
		}
		holds = append(holds, hold)
	}
	return holds, rows.Err()
}

func (r *Repository) SumActiveHolds(ctx context.Context, merchantID, accountCode, currency string) (int64, error) {
	var amount int64
	err := r.db.QueryRow(ctx, `
SELECT COALESCE(SUM(amount), 0)
FROM paygate_ledger.ledger_holds
WHERE merchant_id = $1
  AND account_code = $2
  AND currency = $3
  AND status = 'active'
  AND (expires_at IS NULL OR expires_at > NOW())
`, merchantID, accountCode, currency).Scan(&amount)
	if err != nil {
		return 0, fmt.Errorf("sum active holds: %w", err)
	}
	return amount, nil
}

func (r *Repository) ReleaseHold(ctx context.Context, merchantID, holdID string) (Hold, error) {
	row := r.db.QueryRow(ctx, `
UPDATE paygate_ledger.ledger_holds
SET status = 'released', released_at = NOW(), updated_at = NOW()
WHERE id = $1 AND merchant_id = $2 AND status = 'active'
RETURNING id, merchant_id, account_code, source_type, source_id, reason, currency, amount, status,
          idempotency_key, target_account_code, description, expires_at, released_at, committed_at, created_at, updated_at
`, holdID, merchantID)
	hold, err := scanHold(row)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, getErr := r.GetHold(ctx, merchantID, holdID)
		if getErr != nil {
			return Hold{}, getErr
		}
		if existing.Status == HoldStatusReleased {
			return existing, nil
		}
		if existing.Status != HoldStatusActive {
			return Hold{}, ErrHoldNotActive
		}
		return Hold{}, err
	}
	if err != nil {
		return Hold{}, fmt.Errorf("release ledger hold: %w", err)
	}
	return hold, nil
}

func (r *Repository) ExtendHold(ctx context.Context, in ExtendHoldInput) (Hold, error) {
	row := r.db.QueryRow(ctx, `
UPDATE paygate_ledger.ledger_holds
SET expires_at = $3, updated_at = NOW()
WHERE id = $1 AND merchant_id = $2 AND status = 'active'
RETURNING id, merchant_id, account_code, source_type, source_id, reason, currency, amount, status,
          idempotency_key, target_account_code, description, expires_at, released_at, committed_at, created_at, updated_at
`, in.HoldID, in.MerchantID, in.ExpiresAt)
	hold, err := scanHold(row)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, getErr := r.GetHold(ctx, in.MerchantID, in.HoldID)
		if getErr != nil {
			return Hold{}, getErr
		}
		if existing.Status != HoldStatusActive {
			return Hold{}, ErrHoldNotActive
		}
		return Hold{}, err
	}
	if err != nil {
		return Hold{}, fmt.Errorf("extend ledger hold: %w", err)
	}
	return hold, nil
}

func (r *Repository) CommitHold(ctx context.Context, in CommitHoldInput) (Hold, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Hold{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var hold Hold
	row := tx.QueryRow(ctx, `
SELECT id, merchant_id, account_code, source_type, source_id, reason, currency, amount, status,
       idempotency_key, target_account_code, description, expires_at, released_at, committed_at, created_at, updated_at
FROM paygate_ledger.ledger_holds
WHERE id = $1 AND merchant_id = $2
FOR UPDATE
`, in.HoldID, in.MerchantID)
	hold, err = scanHold(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Hold{}, ErrHoldNotFound
	}
	if err != nil {
		return Hold{}, err
	}
	if hold.Status == HoldStatusCommitted {
		return hold, nil
	}
	if hold.Status != HoldStatusActive {
		return Hold{}, ErrHoldNotActive
	}

	entries := []Entry{
		{AccountCode: hold.AccountCode, DebitAmount: hold.Amount, Currency: hold.Currency, Description: in.Description},
		{AccountCode: in.TargetAccountCode, CreditAmount: hold.Amount, Currency: hold.Currency, Description: in.Description},
	}
	if err := ValidateEntries(entries); err != nil {
		return Hold{}, err
	}
	txnID := idgen.New("txn")
	if err := r.InsertTransactionWithEntriesTx(ctx, tx, txnID, hold.MerchantID, "ledger_hold_commit", hold.ID, in.Description, entries); err != nil {
		return Hold{}, err
	}

	row = tx.QueryRow(ctx, `
UPDATE paygate_ledger.ledger_holds
SET status = 'committed',
    target_account_code = $3,
    description = $4,
    committed_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
RETURNING id, merchant_id, account_code, source_type, source_id, reason, currency, amount, status,
          idempotency_key, target_account_code, description, expires_at, released_at, committed_at, created_at, updated_at
`, in.HoldID, in.MerchantID, in.TargetAccountCode, in.Description)
	hold, err = scanHold(row)
	if err != nil {
		return Hold{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Hold{}, err
	}
	return hold, nil
}

func (r *Repository) ExpireHold(ctx context.Context, merchantID, holdID string) (Hold, error) {
	row := r.db.QueryRow(ctx, `
UPDATE paygate_ledger.ledger_holds
SET status = 'expired', updated_at = NOW()
WHERE id = $1 AND merchant_id = $2 AND status = 'active'
RETURNING id, merchant_id, account_code, source_type, source_id, reason, currency, amount, status,
          idempotency_key, target_account_code, description, expires_at, released_at, committed_at, created_at, updated_at
`, holdID, merchantID)
	hold, err := scanHold(row)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, getErr := r.GetHold(ctx, merchantID, holdID)
		if getErr != nil {
			return Hold{}, getErr
		}
		if existing.Status == HoldStatusExpired {
			return existing, nil
		}
		if existing.Status != HoldStatusActive {
			return Hold{}, ErrHoldNotActive
		}
		return Hold{}, err
	}
	if err != nil {
		return Hold{}, fmt.Errorf("expire ledger hold: %w", err)
	}
	return hold, nil
}

func (r *Repository) ExpireDueHolds(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	tag, err := r.db.Exec(ctx, `
WITH expired AS (
    SELECT id
    FROM paygate_ledger.ledger_holds
    WHERE status = 'active' AND expires_at IS NOT NULL AND expires_at <= NOW()
    ORDER BY expires_at ASC
    LIMIT $1
)
UPDATE paygate_ledger.ledger_holds h
SET status = 'expired', updated_at = NOW()
FROM expired e
WHERE h.id = e.id
`, limit)
	if err != nil {
		return 0, fmt.Errorf("expire ledger holds: %w", err)
	}
	return tag.RowsAffected(), nil
}

type holdScannable interface {
	Scan(dest ...any) error
}

func scanHold(row holdScannable) (Hold, error) {
	var hold Hold
	var targetAccountCode *string
	var description *string
	err := row.Scan(
		&hold.ID, &hold.MerchantID, &hold.AccountCode, &hold.SourceType, &hold.SourceID, &hold.Reason,
		&hold.Currency, &hold.Amount, &hold.Status, &hold.IdempotencyKey, &targetAccountCode, &description,
		&hold.ExpiresAt, &hold.ReleasedAt, &hold.CommittedAt, &hold.CreatedAt, &hold.UpdatedAt,
	)
	if err != nil {
		return Hold{}, err
	}
	if targetAccountCode != nil {
		hold.TargetAccountCode = *targetAccountCode
	}
	if description != nil {
		hold.Description = *description
	}
	return hold, nil
}
