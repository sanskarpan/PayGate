package settlement

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

// RunBatch collects all captured, non-settled payments for the merchant in [periodStart, periodEnd),
// creates a settlement + items, writes ledger entries, marks payments settled, and writes outbox events.
func (r *PostgresRepository) RunBatch(ctx context.Context, merchantID string, periodStart, periodEnd time.Time) (Settlement, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Settlement{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Collect eligible payments (captured, not yet settled, within period).
	rows, err := tx.Query(ctx, `
SELECT id, amount, fee, amount_refunded, currency
FROM paygate_payments.payments
WHERE merchant_id = $1
  AND status = 'captured'
  AND settled = false
  AND captured_at >= $2
  AND captured_at < $3
FOR UPDATE SKIP LOCKED
`, merchantID, periodStart, periodEnd)
	if err != nil {
		return Settlement{}, fmt.Errorf("query eligible payments: %w", err)
	}
	defer rows.Close()

	var payments []EligiblePayment
	for rows.Next() {
		var p EligiblePayment
		if err := rows.Scan(&p.PaymentID, &p.Amount, &p.Fee, &p.AmountRefunded, &p.Currency); err != nil {
			return Settlement{}, fmt.Errorf("scan payment: %w", err)
		}
		payments = append(payments, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Settlement{}, err
	}
	if len(payments) == 0 {
		return Settlement{}, ErrNoEligiblePayments
	}

	// Aggregate totals.
	var totalAmount, totalFees, totalRefunds int64
	currency := payments[0].Currency
	for _, p := range payments {
		totalAmount += p.Amount
		totalFees += p.Fee
		totalRefunds += p.AmountRefunded
	}
	netAmount := CalculateNet(totalAmount, totalFees, totalRefunds)

	// Create settlement record.
	sttlID := idgen.New("sttl")
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_settlements.settlements
    (id, merchant_id, status, period_start, period_end, total_amount, total_fees,
     total_refunds, net_amount, payment_count, currency, processed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
`, sttlID, merchantID, StateProcessed, periodStart, periodEnd,
		totalAmount, totalFees, totalRefunds, netAmount, len(payments), currency, now,
	); err != nil {
		return Settlement{}, fmt.Errorf("insert settlement: %w", err)
	}

	// Create settlement items and collect payment IDs.
	paymentIDs := make([]string, 0, len(payments))
	for _, p := range payments {
		net := CalculateNet(p.Amount, p.Fee, p.AmountRefunded)
		if _, err := tx.Exec(ctx, `
INSERT INTO paygate_settlements.settlement_items
    (id, settlement_id, payment_id, merchant_id, amount, fee, refunds, net, currency)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`, idgen.New("si"), sttlID, p.PaymentID, merchantID, p.Amount, p.Fee, p.AmountRefunded, net, p.Currency,
		); err != nil {
			return Settlement{}, fmt.Errorf("insert settlement item: %w", err)
		}
		paymentIDs = append(paymentIDs, p.PaymentID)
	}

	// Write double-entry ledger: Dr. MERCHANT_PAYABLE / Cr. SETTLEMENT_CLEARING
	if _, err := r.ledger.CreateEntriesTx(ctx, tx, merchantID, "settlement", sttlID,
		fmt.Sprintf("settlement batch %s", sttlID),
		[]ledger.Entry{
			{AccountCode: "MERCHANT_PAYABLE", DebitAmount: netAmount, Description: "merchant payout on settlement"},
			{AccountCode: "SETTLEMENT_CLEARING", CreditAmount: netAmount, Description: "settlement clearing"},
		},
	); err != nil {
		return Settlement{}, fmt.Errorf("write settlement ledger entries: %w", err)
	}

	// Mark payments as settled.
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET settled = true, settlement_id = $1, updated_at = NOW()
WHERE id = ANY($2)
`, sttlID, paymentIDs); err != nil {
		return Settlement{}, fmt.Errorf("mark payments settled: %w", err)
	}

	// Write outbox events.
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "settlement",
		AggregateID:   sttlID,
		EventType:     "settlement.processed",
		MerchantID:    merchantID,
		Payload: map[string]any{
			"settlement_id": sttlID,
			"net_amount":    netAmount,
			"payment_count": len(payments),
			"currency":      currency,
		},
	}); err != nil {
		return Settlement{}, fmt.Errorf("write settlement outbox event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Settlement{}, err
	}

	return r.GetSettlement(ctx, merchantID, sttlID)
}

func (r *PostgresRepository) GetSettlement(ctx context.Context, merchantID, id string) (Settlement, error) {
	var s Settlement
	err := r.db.QueryRow(ctx, `
SELECT id, merchant_id, status, period_start, period_end, total_amount, total_fees,
       total_refunds, net_amount, payment_count, currency, processed_at, created_at, updated_at
FROM paygate_settlements.settlements
WHERE id = $1 AND merchant_id = $2
`, id, merchantID).Scan(
		&s.ID, &s.MerchantID, &s.Status, &s.PeriodStart, &s.PeriodEnd,
		&s.TotalAmount, &s.TotalFees, &s.TotalRefunds, &s.NetAmount,
		&s.PaymentCount, &s.Currency, &s.ProcessedAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settlement{}, ErrSettlementNotFound
	}
	return s, err
}

func (r *PostgresRepository) ListSettlements(ctx context.Context, merchantID string) ([]Settlement, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, status, period_start, period_end, total_amount, total_fees,
       total_refunds, net_amount, payment_count, currency, processed_at, created_at, updated_at
FROM paygate_settlements.settlements
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT 100
`, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settlements []Settlement
	for rows.Next() {
		var s Settlement
		if err := rows.Scan(
			&s.ID, &s.MerchantID, &s.Status, &s.PeriodStart, &s.PeriodEnd,
			&s.TotalAmount, &s.TotalFees, &s.TotalRefunds, &s.NetAmount,
			&s.PaymentCount, &s.Currency, &s.ProcessedAt, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		settlements = append(settlements, s)
	}
	return settlements, rows.Err()
}

func (r *PostgresRepository) GetSettlementItems(ctx context.Context, settlementID string) ([]SettlementItem, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, settlement_id, payment_id, merchant_id, amount, fee, refunds, net, currency, created_at
FROM paygate_settlements.settlement_items
WHERE settlement_id = $1
ORDER BY created_at
`, settlementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SettlementItem
	for rows.Next() {
		var item SettlementItem
		if err := rows.Scan(&item.ID, &item.SettlementID, &item.PaymentID, &item.MerchantID,
			&item.Amount, &item.Fee, &item.Refunds, &item.Net, &item.Currency, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
