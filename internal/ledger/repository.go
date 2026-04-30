package ledger

import (
	"context"
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
	for _, e := range entries {
		totalDebit += e.DebitAmount
		totalCredit += e.CreditAmount
	}

	_, err := tx.Exec(ctx, `
INSERT INTO paygate_ledger.ledger_transactions
(id, merchant_id, source_type, source_id, currency, total_debit, total_credit, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`, txnID, merchantID, sourceType, sourceID, "INR", totalDebit, totalCredit, description)
	if err != nil {
		return fmt.Errorf("insert ledger transaction: %w", err)
	}

	for _, e := range entries {
		_, err := tx.Exec(ctx, `
INSERT INTO paygate_ledger.ledger_entries
(id, transaction_id, merchant_id, account_code, debit_amount, credit_amount, currency, source_type, source_id, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
`, idgen.New("le"), txnID, merchantID, e.AccountCode, e.DebitAmount, e.CreditAmount, "INR", sourceType, sourceID, e.Description)
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
