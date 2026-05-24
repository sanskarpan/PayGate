package settlement

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/outbox"
)

// PostgresRepository implements Repository using pgxpool.
type PostgresRepository struct {
	db                    *pgxpool.Pool
	ledger                *ledger.Service
	outbox                *outbox.Writer
	reservePolicyResolver interface {
		GetReservePolicy(ctx context.Context, merchantID string) (merchant.ReservePolicy, error)
	}
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(db *pgxpool.Pool, ledgerSvc *ledger.Service) *PostgresRepository {
	return &PostgresRepository{db: db, ledger: ledgerSvc, outbox: outbox.NewWriter()}
}

func (r *PostgresRepository) SetReservePolicyResolver(resolver interface {
	GetReservePolicy(ctx context.Context, merchantID string) (merchant.ReservePolicy, error)
}) {
	r.reservePolicyResolver = resolver
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
		totalRefunds += CalculateRefundNetImpact(p.Amount, p.Fee, p.AmountRefunded)
	}
	grossNetAmount := CalculateNet(totalAmount, totalFees, totalRefunds)
	reserveAmount := r.calculateReserveAmount(ctx, merchantID, grossNetAmount)
	netAmount := grossNetAmount - reserveAmount

	// Create settlement record.
	sttlID := idgen.New("sttl")
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_settlements.settlements
    (id, merchant_id, status, period_start, period_end, total_amount, total_fees,
     total_refunds, gross_net_amount, reserve_amount, net_amount, payment_count, currency, processed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
`, sttlID, merchantID, StateProcessed, periodStart, periodEnd,
		totalAmount, totalFees, totalRefunds, grossNetAmount, reserveAmount, netAmount, len(payments), currency, now,
	); err != nil {
		return Settlement{}, fmt.Errorf("insert settlement: %w", err)
	}

	// Create settlement items and collect payment IDs.
	paymentIDs := make([]string, 0, len(payments))
	for _, p := range payments {
		net := CalculateNet(p.Amount, p.Fee, CalculateRefundNetImpact(p.Amount, p.Fee, p.AmountRefunded))
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
		r.settlementEntries(grossNetAmount, reserveAmount, netAmount),
	); err != nil {
		return Settlement{}, fmt.Errorf("write settlement ledger entries: %w", err)
	}
	if reserveAmount > 0 {
		releaseAt := now.Add(time.Duration(r.reserveHoldDays(ctx, merchantID)) * 24 * time.Hour)
		if _, err := tx.Exec(ctx, `
INSERT INTO paygate_settlements.settlement_reserve_releases
    (id, merchant_id, settlement_id, amount, release_at, status)
VALUES ($1, $2, $3, $4, $5, 'scheduled')
`, idgen.New("srel"), merchantID, sttlID, reserveAmount, releaseAt); err != nil {
			return Settlement{}, fmt.Errorf("insert settlement reserve release: %w", err)
		}
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
			"settlement_id":  sttlID,
			"net_amount":     netAmount,
			"reserve_amount": reserveAmount,
			"payment_count":  len(payments),
			"currency":       currency,
		},
	}); err != nil {
		return Settlement{}, fmt.Errorf("write settlement outbox event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Settlement{}, err
	}

	return r.GetSettlement(ctx, merchantID, sttlID)
}

// RunPartialBatch collects only the specified captured, non-settled payments for the merchant,
// creates a settlement + items, writes ledger entries, marks payments settled, and writes outbox events.
// The settlement period is derived from the min/max captured_at of the selected payments.
func (r *PostgresRepository) RunPartialBatch(ctx context.Context, merchantID string, paymentIDs []string) (Settlement, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Settlement{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Collect eligible payments matching the provided IDs.
	rows, err := tx.Query(ctx, `
SELECT id, amount, fee, amount_refunded, currency
FROM paygate_payments.payments
WHERE merchant_id = $1
  AND status = 'captured'
  AND settled = false
  AND id = ANY($2)
FOR UPDATE SKIP LOCKED
`, merchantID, paymentIDs)
	if err != nil {
		return Settlement{}, fmt.Errorf("query partial payments: %w", err)
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
		totalRefunds += CalculateRefundNetImpact(p.Amount, p.Fee, p.AmountRefunded)
	}
	grossNetAmount := CalculateNet(totalAmount, totalFees, totalRefunds)
	reserveAmount := r.calculateReserveAmount(ctx, merchantID, grossNetAmount)
	netAmount := grossNetAmount - reserveAmount

	// Derive period from the min/max captured_at of selected payments.
	var periodStart, periodEnd time.Time
	err = tx.QueryRow(ctx, `
SELECT MIN(captured_at), MAX(captured_at)
FROM paygate_payments.payments
WHERE id = ANY($1)
`, paymentIDs).Scan(&periodStart, &periodEnd)
	if err != nil {
		return Settlement{}, fmt.Errorf("query period bounds: %w", err)
	}

	// Create settlement record.
	sttlID := idgen.New("sttl")
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_settlements.settlements
    (id, merchant_id, status, period_start, period_end, total_amount, total_fees,
     total_refunds, gross_net_amount, reserve_amount, net_amount, payment_count, currency, processed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
`, sttlID, merchantID, StateProcessed, periodStart, periodEnd,
		totalAmount, totalFees, totalRefunds, grossNetAmount, reserveAmount, netAmount, len(payments), currency, now,
	); err != nil {
		return Settlement{}, fmt.Errorf("insert settlement: %w", err)
	}

	// Create settlement items and collect payment IDs.
	selectedIDs := make([]string, 0, len(payments))
	for _, p := range payments {
		net := CalculateNet(p.Amount, p.Fee, CalculateRefundNetImpact(p.Amount, p.Fee, p.AmountRefunded))
		if _, err := tx.Exec(ctx, `
INSERT INTO paygate_settlements.settlement_items
    (id, settlement_id, payment_id, merchant_id, amount, fee, refunds, net, currency)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`, idgen.New("si"), sttlID, p.PaymentID, merchantID, p.Amount, p.Fee, p.AmountRefunded, net, p.Currency,
		); err != nil {
			return Settlement{}, fmt.Errorf("insert settlement item: %w", err)
		}
		selectedIDs = append(selectedIDs, p.PaymentID)
	}

	// Write double-entry ledger: Dr. MERCHANT_PAYABLE / Cr. SETTLEMENT_CLEARING
	if _, err := r.ledger.CreateEntriesTx(ctx, tx, merchantID, "settlement", sttlID,
		fmt.Sprintf("partial settlement batch %s", sttlID),
		r.settlementEntries(grossNetAmount, reserveAmount, netAmount),
	); err != nil {
		return Settlement{}, fmt.Errorf("write settlement ledger entries: %w", err)
	}
	if reserveAmount > 0 {
		releaseAt := now.Add(time.Duration(r.reserveHoldDays(ctx, merchantID)) * 24 * time.Hour)
		if _, err := tx.Exec(ctx, `
INSERT INTO paygate_settlements.settlement_reserve_releases
    (id, merchant_id, settlement_id, amount, release_at, status)
VALUES ($1, $2, $3, $4, $5, 'scheduled')
`, idgen.New("srel"), merchantID, sttlID, reserveAmount, releaseAt); err != nil {
			return Settlement{}, fmt.Errorf("insert settlement reserve release: %w", err)
		}
	}

	// Mark payments as settled.
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payments.payments
SET settled = true, settlement_id = $1, updated_at = NOW()
WHERE id = ANY($2)
`, sttlID, selectedIDs); err != nil {
		return Settlement{}, fmt.Errorf("mark payments settled: %w", err)
	}

	// Write outbox events.
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "settlement",
		AggregateID:   sttlID,
		EventType:     "settlement.processed",
		MerchantID:    merchantID,
		Payload: map[string]any{
			"settlement_id":  sttlID,
			"net_amount":     netAmount,
			"reserve_amount": reserveAmount,
			"payment_count":  len(payments),
			"currency":       currency,
			"partial":        true,
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
	var holdReason *string
	var rollbackReason *string
	err := r.db.QueryRow(ctx, `
SELECT id, merchant_id, status, period_start, period_end, total_amount, total_fees,
       total_refunds, gross_net_amount, reserve_amount, net_amount, payment_count, currency, processed_at, created_at, updated_at,
       on_hold, hold_reason, held_at, released_at, rollback_marked_at, rollback_reason
FROM paygate_settlements.settlements
WHERE id = $1 AND merchant_id = $2
`, id, merchantID).Scan(
		&s.ID, &s.MerchantID, &s.Status, &s.PeriodStart, &s.PeriodEnd,
		&s.TotalAmount, &s.TotalFees, &s.TotalRefunds, &s.GrossNetAmount, &s.ReserveAmount, &s.NetAmount,
		&s.PaymentCount, &s.Currency, &s.ProcessedAt, &s.CreatedAt, &s.UpdatedAt,
		&s.OnHold, &holdReason, &s.HeldAt, &s.ReleasedAt, &s.RollbackMarkedAt, &rollbackReason,
	)
	if holdReason != nil {
		s.HoldReason = *holdReason
	}
	if rollbackReason != nil {
		s.RollbackReason = *rollbackReason
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Settlement{}, ErrSettlementNotFound
	}
	return s, err
}

func (r *PostgresRepository) ListSettlements(ctx context.Context, merchantID string) ([]Settlement, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, status, period_start, period_end, total_amount, total_fees,
       total_refunds, gross_net_amount, reserve_amount, net_amount, payment_count, currency, processed_at, created_at, updated_at,
       on_hold, hold_reason, held_at, released_at, rollback_marked_at, rollback_reason
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
		var holdReason *string
		var rollbackReason *string
		if err := rows.Scan(
			&s.ID, &s.MerchantID, &s.Status, &s.PeriodStart, &s.PeriodEnd,
			&s.TotalAmount, &s.TotalFees, &s.TotalRefunds, &s.GrossNetAmount, &s.ReserveAmount, &s.NetAmount,
			&s.PaymentCount, &s.Currency, &s.ProcessedAt, &s.CreatedAt, &s.UpdatedAt,
			&s.OnHold, &holdReason, &s.HeldAt, &s.ReleasedAt, &s.RollbackMarkedAt, &rollbackReason,
		); err != nil {
			return nil, err
		}
		if holdReason != nil {
			s.HoldReason = *holdReason
		}
		if rollbackReason != nil {
			s.RollbackReason = *rollbackReason
		}
		settlements = append(settlements, s)
	}
	return settlements, rows.Err()
}

func (r *PostgresRepository) HoldSettlement(ctx context.Context, merchantID, settlementID, reason string) error {
	tag, err := r.db.Exec(ctx, `
UPDATE paygate_settlements.settlements
SET on_hold = TRUE, hold_reason = $3, held_at = NOW(), updated_at = NOW()
WHERE id = $1 AND merchant_id = $2 AND on_hold = FALSE
`, settlementID, merchantID, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSettlementOnHold
	}
	return nil
}

func (r *PostgresRepository) ReleaseSettlement(ctx context.Context, merchantID, settlementID string) error {
	tag, err := r.db.Exec(ctx, `
UPDATE paygate_settlements.settlements
SET on_hold = FALSE, hold_reason = NULL, released_at = NOW(), updated_at = NOW()
WHERE id = $1 AND merchant_id = $2 AND on_hold = TRUE
`, settlementID, merchantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSettlementNotOnHold
	}
	return nil
}

func (r *PostgresRepository) GetSettlementItems(ctx context.Context, settlementID string) ([]SettlementItem, error) {
	rows, err := r.db.Query(ctx, `
SELECT si.id, si.settlement_id, si.payment_id, si.merchant_id, si.amount, si.fee, si.refunds, si.net, si.currency,
       COALESCE((
           SELECT jsonb_agg(jsonb_build_object(
               'destination_type', ps.destination_type,
               'destination_ref', ps.destination_ref,
               'beneficiary_label', ps.beneficiary_label,
               'amount', ps.amount,
               'currency', ps.currency
           ) ORDER BY ps.created_at, ps.id)
           FROM paygate_payments.payment_splits ps
           WHERE ps.payment_id = si.payment_id
       ), '[]'::jsonb) AS split_summary,
       si.created_at
FROM paygate_settlements.settlement_items si
WHERE si.settlement_id = $1
ORDER BY created_at
`, settlementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SettlementItem
	for rows.Next() {
		var item SettlementItem
		var splitSummary []byte
		if err := rows.Scan(&item.ID, &item.SettlementID, &item.PaymentID, &item.MerchantID,
			&item.Amount, &item.Fee, &item.Refunds, &item.Net, &item.Currency, &splitSummary, &item.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(splitSummary, &item.SplitSummary)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) MarkRollback(ctx context.Context, merchantID, settlementID, reason string) (Settlement, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Settlement{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current Settlement
	var holdReason *string
	var rollbackReason *string
	err = tx.QueryRow(ctx, `
SELECT id, merchant_id, status, period_start, period_end, total_amount, total_fees,
       total_refunds, gross_net_amount, reserve_amount, net_amount, payment_count, currency, processed_at, created_at, updated_at,
       on_hold, hold_reason, held_at, released_at, rollback_marked_at, rollback_reason
FROM paygate_settlements.settlements
WHERE id = $1 AND merchant_id = $2
FOR UPDATE
`, settlementID, merchantID).Scan(
		&current.ID, &current.MerchantID, &current.Status, &current.PeriodStart, &current.PeriodEnd,
		&current.TotalAmount, &current.TotalFees, &current.TotalRefunds, &current.GrossNetAmount, &current.ReserveAmount, &current.NetAmount,
		&current.PaymentCount, &current.Currency, &current.ProcessedAt, &current.CreatedAt, &current.UpdatedAt,
		&current.OnHold, &holdReason, &current.HeldAt, &current.ReleasedAt, &current.RollbackMarkedAt, &rollbackReason,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Settlement{}, ErrSettlementNotFound
		}
		return Settlement{}, err
	}
	if holdReason != nil {
		current.HoldReason = *holdReason
	}
	if rollbackReason != nil {
		current.RollbackReason = *rollbackReason
	}
	if current.RollbackMarkedAt != nil {
		return current, nil
	}

	now := time.Now().UTC()
	holdReasonValue := current.HoldReason
	if holdReasonValue == "" {
		holdReasonValue = "rollback marker"
		if reason != "" {
			holdReasonValue = "rollback marker: " + reason
		}
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_settlements.settlements
SET rollback_marked_at = $3,
    rollback_reason = NULLIF($4, ''),
    on_hold = TRUE,
    hold_reason = $5,
    held_at = COALESCE(held_at, $3),
    updated_at = $3
WHERE id = $1 AND merchant_id = $2
`, settlementID, merchantID, now, reason, holdReasonValue); err != nil {
		return Settlement{}, err
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "settlement",
		AggregateID:   settlementID,
		EventType:     "settlement.rollback_marked",
		MerchantID:    merchantID,
		Payload: map[string]any{
			"settlement_id": settlementID,
			"reason":        reason,
		},
	}); err != nil {
		return Settlement{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Settlement{}, err
	}
	return r.GetSettlement(ctx, merchantID, settlementID)
}

func (r *PostgresRepository) GetPreferences(ctx context.Context, merchantID string) (Preferences, error) {
	var prefs Preferences
	err := r.db.QueryRow(ctx, `
SELECT merchant_id, schedule_type, weekly_day_of_week, payout_minimum, approval_threshold_amount, weekend_policy, auto_payout, created_at, updated_at
FROM paygate_settlements.settlement_preferences
WHERE merchant_id = $1
`, merchantID).Scan(&prefs.MerchantID, &prefs.ScheduleType, &prefs.WeeklyDayOfWeek, &prefs.PayoutMinimum, &prefs.ApprovalThresholdAmount, &prefs.WeekendPolicy, &prefs.AutoPayout, &prefs.CreatedAt, &prefs.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultPreferences(merchantID), nil
	}
	return prefs, err
}

func (r *PostgresRepository) UpsertPreferences(ctx context.Context, prefs Preferences) (Preferences, error) {
	err := r.db.QueryRow(ctx, `
INSERT INTO paygate_settlements.settlement_preferences
    (merchant_id, schedule_type, weekly_day_of_week, payout_minimum, approval_threshold_amount, weekend_policy, auto_payout)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (merchant_id)
DO UPDATE SET schedule_type = EXCLUDED.schedule_type,
              weekly_day_of_week = EXCLUDED.weekly_day_of_week,
              payout_minimum = EXCLUDED.payout_minimum,
              approval_threshold_amount = EXCLUDED.approval_threshold_amount,
              weekend_policy = EXCLUDED.weekend_policy,
              auto_payout = EXCLUDED.auto_payout,
              updated_at = NOW()
RETURNING merchant_id, schedule_type, weekly_day_of_week, payout_minimum, approval_threshold_amount, weekend_policy, auto_payout, created_at, updated_at
`, prefs.MerchantID, prefs.ScheduleType, prefs.WeeklyDayOfWeek, prefs.PayoutMinimum, prefs.ApprovalThresholdAmount, prefs.WeekendPolicy, prefs.AutoPayout).Scan(
		&prefs.MerchantID, &prefs.ScheduleType, &prefs.WeeklyDayOfWeek, &prefs.PayoutMinimum, &prefs.ApprovalThresholdAmount, &prefs.WeekendPolicy, &prefs.AutoPayout, &prefs.CreatedAt, &prefs.UpdatedAt,
	)
	return prefs, err
}

func (r *PostgresRepository) GenerateStatement(ctx context.Context, merchantID, settlementID string) (Statement, error) {
	sttl, err := r.GetSettlement(ctx, merchantID, settlementID)
	if err != nil {
		return Statement{}, err
	}
	items, err := r.GetSettlementItems(ctx, settlementID)
	if err != nil {
		return Statement{}, err
	}
	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	_ = writer.Write([]string{"payment_id", "gross_amount", "fee", "refunds", "net", "currency"})
	for _, item := range items {
		_ = writer.Write([]string{
			item.PaymentID,
			fmt.Sprintf("%d", item.Amount),
			fmt.Sprintf("%d", item.Fee),
			fmt.Sprintf("%d", item.Refunds),
			fmt.Sprintf("%d", item.Net),
			item.Currency,
		})
	}
	writer.Flush()
	content := builder.String()
	totals := map[string]any{
		"gross_net_amount": sttl.GrossNetAmount,
		"reserve_amount":   sttl.ReserveAmount,
		"net_amount":       sttl.NetAmount,
		"payment_count":    sttl.PaymentCount,
	}
	raw, err := json.Marshal(totals)
	if err != nil {
		return Statement{}, err
	}
	var stmt Statement
	err = r.db.QueryRow(ctx, `
INSERT INTO paygate_settlements.settlement_statements
    (id, merchant_id, settlement_id, format, file_name, content, totals_json)
VALUES ($1, $2, $3, 'csv', $4, $5, $6)
RETURNING id, merchant_id, settlement_id, format, file_name, content, totals_json, created_at
`, idgen.New("stmt"), merchantID, settlementID, fmt.Sprintf("%s.csv", settlementID), content, raw).Scan(
		&stmt.ID, &stmt.MerchantID, &stmt.SettlementID, &stmt.Format, &stmt.FileName, &stmt.Content, &raw, &stmt.CreatedAt,
	)
	if err != nil {
		return Statement{}, err
	}
	_ = json.Unmarshal(raw, &stmt.Totals)
	return stmt, nil
}

func (r *PostgresRepository) GetStatement(ctx context.Context, merchantID, settlementID string) (Statement, error) {
	var stmt Statement
	var raw []byte
	err := r.db.QueryRow(ctx, `
SELECT id, merchant_id, settlement_id, format, file_name, content, totals_json, created_at
FROM paygate_settlements.settlement_statements
WHERE merchant_id = $1 AND settlement_id = $2
ORDER BY created_at DESC
LIMIT 1
`, merchantID, settlementID).Scan(&stmt.ID, &stmt.MerchantID, &stmt.SettlementID, &stmt.Format, &stmt.FileName, &stmt.Content, &raw, &stmt.CreatedAt)
	if err != nil {
		return Statement{}, err
	}
	_ = json.Unmarshal(raw, &stmt.Totals)
	return stmt, nil
}

func (r *PostgresRepository) ListAdjustments(ctx context.Context, merchantID, settlementID string) ([]Adjustment, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, settlement_id, payment_id, refund_id, adjustment_type, amount, currency, reason, created_at
FROM paygate_settlements.settlement_adjustments
WHERE merchant_id = $1 AND settlement_id = $2
ORDER BY created_at DESC
`, merchantID, settlementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Adjustment
	for rows.Next() {
		var item Adjustment
		if err := rows.Scan(&item.ID, &item.MerchantID, &item.SettlementID, &item.PaymentID, &item.RefundID, &item.AdjustmentType, &item.Amount, &item.Currency, &item.Reason, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) RecordAdjustment(ctx context.Context, adjustment Adjustment) error {
	_, err := r.db.Exec(ctx, `
INSERT INTO paygate_settlements.settlement_adjustments
    (id, merchant_id, settlement_id, payment_id, refund_id, adjustment_type, amount, currency, reason)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`, adjustment.ID, adjustment.MerchantID, adjustment.SettlementID, adjustment.PaymentID, adjustment.RefundID, adjustment.AdjustmentType, adjustment.Amount, adjustment.Currency, adjustment.Reason)
	return err
}

func (r *PostgresRepository) calculateReserveAmount(ctx context.Context, merchantID string, grossNetAmount int64) int64 {
	if r.reservePolicyResolver == nil || grossNetAmount <= 0 {
		return 0
	}
	policy, err := r.reservePolicyResolver.GetReservePolicy(ctx, merchantID)
	if err != nil {
		return 0
	}
	return policy.CalculateReserve(grossNetAmount)
}

func (r *PostgresRepository) reserveHoldDays(ctx context.Context, merchantID string) int {
	if r.reservePolicyResolver == nil {
		return 0
	}
	policy, err := r.reservePolicyResolver.GetReservePolicy(ctx, merchantID)
	if err != nil {
		return 0
	}
	return policy.HoldDays
}

func (r *PostgresRepository) settlementEntries(grossNetAmount, reserveAmount, payoutAmount int64) []ledger.Entry {
	entries := []ledger.Entry{
		{AccountCode: "MERCHANT_PAYABLE", DebitAmount: grossNetAmount, Description: "merchant payout on settlement"},
	}
	if payoutAmount > 0 {
		entries = append(entries, ledger.Entry{AccountCode: "SETTLEMENT_CLEARING", CreditAmount: payoutAmount, Description: "settlement clearing"})
	}
	if reserveAmount > 0 {
		entries = append(entries, ledger.Entry{AccountCode: "MERCHANT_RESERVE_HELD", CreditAmount: reserveAmount, Description: "merchant reserve hold"})
	}
	return entries
}
