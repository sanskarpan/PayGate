package recon

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/settlement"
)

// Worker runs periodic reconciliation checks against the Postgres DB.
// Three check types:
//
//	LedgerBalance: every 5 min — sum debits == credits for all merchants
//	PaymentLedger: hourly — every captured payment has matching ledger entries
//	ThreeWay:      nightly — settled payments appear in settlement_items with matching amounts
type Worker struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

// NewWorker creates a Worker.
func NewWorker(db *pgxpool.Pool, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{db: db, logger: logger}
}

// Start launches all three reconciliation schedules until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) {
	ledgerTicker := time.NewTicker(5 * time.Minute)
	paymentTicker := time.NewTicker(time.Hour)
	threeWayTicker := time.NewTicker(24 * time.Hour)

	defer ledgerTicker.Stop()
	defer paymentTicker.Stop()
	defer threeWayTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ledgerTicker.C:
			if n, err := w.RunLedgerBalanceCheck(ctx); err != nil {
				w.logger.Error("ledger balance check failed", "error", err)
			} else if n > 0 {
				w.logger.Warn("ledger balance mismatches detected", "count", n)
			}
		case <-paymentTicker.C:
			if n, err := w.RunPaymentLedgerCheck(ctx, time.Now().Add(-time.Hour), time.Now()); err != nil {
				w.logger.Error("payment-ledger check failed", "error", err)
			} else if n > 0 {
				w.logger.Warn("payment-ledger mismatches detected", "count", n)
			}
		case <-threeWayTicker.C:
			yesterday := time.Now().Truncate(24 * time.Hour).Add(-24 * time.Hour)
			today := yesterday.Add(24 * time.Hour)
			if n, err := w.RunThreeWayCheck(ctx, yesterday, today); err != nil {
				w.logger.Error("three-way recon failed", "error", err)
			} else if n > 0 {
				w.logger.Warn("three-way mismatches detected", "count", n)
			}
		}
	}
}

// RunLedgerBalanceCheck verifies total debits == total credits per merchant.
// Returns the number of mismatches found.
func (w *Worker) RunLedgerBalanceCheck(ctx context.Context) (int, error) {
	batchID := idgen.New("recon")
	now := time.Now().UTC()

	// Find merchants where sum(debit_amount) != sum(credit_amount).
	rows, err := w.db.Query(ctx, `
SELECT merchant_id,
       SUM(debit_amount)  AS total_debits,
       SUM(credit_amount) AS total_credits
FROM paygate_ledger.ledger_entries
GROUP BY merchant_id
HAVING SUM(debit_amount) != SUM(credit_amount)
`)
	if err != nil {
		return 0, fmt.Errorf("query ledger balance: %w", err)
	}
	defer rows.Close()

	var mismatches []ReconMismatch
	for rows.Next() {
		var merchantID string
		var totalDebits, totalCredits int64
		if err := rows.Scan(&merchantID, &totalDebits, &totalCredits); err != nil {
			return 0, err
		}
		mismatches = append(mismatches, ReconMismatch{
			ID:            idgen.New("mm"),
			BatchID:       batchID,
			MerchantID:    merchantID,
			MismatchType:  MismatchLedgerImbalance,
			EntityType:    "merchant",
			EntityID:      merchantID,
			ExpectedValue: fmt.Sprintf("debits=%d", totalDebits),
			ActualValue:   fmt.Sprintf("credits=%d", totalCredits),
			Description:   fmt.Sprintf("ledger imbalance: debits=%d credits=%d diff=%d", totalDebits, totalCredits, totalDebits-totalCredits),
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	// Persist batch + mismatches.
	return w.persistBatch(ctx, batchID, "", BatchTypeLedgerBalance, time.Unix(0, 0), now, len(mismatches), len(mismatches), mismatches)
}

// RunPaymentLedgerCheck verifies each captured payment in [start, end) has ledger entries.
func (w *Worker) RunPaymentLedgerCheck(ctx context.Context, start, end time.Time) (int, error) {
	batchID := idgen.New("recon")
	checkedCount, err := w.countCapturedPayments(ctx, start, end)
	if err != nil {
		return 0, fmt.Errorf("count captured payments: %w", err)
	}

	rows, err := w.db.Query(ctx, `
SELECT p.id, p.merchant_id, p.amount,
       COALESCE(SUM(le.credit_amount), 0) AS ledger_credits
FROM paygate_payments.payments p
LEFT JOIN paygate_ledger.ledger_entries le
    ON le.source_id = p.id AND le.source_type = 'payment'
WHERE p.status = 'captured'
  AND p.captured_at >= $1 AND p.captured_at < $2
GROUP BY p.id, p.merchant_id, p.amount
HAVING COALESCE(SUM(le.credit_amount), 0) = 0
    OR ABS(COALESCE(SUM(le.credit_amount), 0) - p.amount) > 0
`, start, end)
	if err != nil {
		return 0, fmt.Errorf("query payment-ledger: %w", err)
	}
	defer rows.Close()

	var mismatches []ReconMismatch
	for rows.Next() {
		var paymentID, merchantID string
		var paymentAmount, ledgerCredits int64
		if err := rows.Scan(&paymentID, &merchantID, &paymentAmount, &ledgerCredits); err != nil {
			return 0, err
		}

		mt := MismatchPaymentAmountMismatch
		if ledgerCredits == 0 {
			mt = MismatchPaymentMissingLedger
		}
		mismatches = append(mismatches, ReconMismatch{
			ID:            idgen.New("mm"),
			BatchID:       batchID,
			MerchantID:    merchantID,
			MismatchType:  mt,
			EntityType:    "payment",
			EntityID:      paymentID,
			ExpectedValue: fmt.Sprintf("%d", paymentAmount),
			ActualValue:   fmt.Sprintf("%d", ledgerCredits),
			Description:   fmt.Sprintf("payment %s: amount=%d ledger_credits=%d", paymentID, paymentAmount, ledgerCredits),
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	return w.persistBatch(ctx, batchID, "", BatchTypePaymentLedger, start, end, checkedCount, len(mismatches), mismatches)
}

// RunThreeWayCheck verifies settled payments have matching settlement_items.
func (w *Worker) RunThreeWayCheck(ctx context.Context, start, end time.Time) (int, error) {
	batchID := idgen.New("recon")
	checkedCount, err := w.countCapturedPayments(ctx, start, end)
	if err != nil {
		return 0, fmt.Errorf("count captured payments: %w", err)
	}

	rows, err := w.db.Query(ctx, `
SELECT p.id, p.merchant_id, p.amount, p.fee, p.amount_refunded, p.settled, p.settlement_id,
       si.id   AS si_id,
       si.net  AS si_net
FROM paygate_payments.payments p
LEFT JOIN paygate_settlements.settlement_items si ON si.payment_id = p.id
WHERE p.status = 'captured'
  AND p.captured_at >= $1 AND p.captured_at < $2
  AND (p.settled = true OR si.id IS NOT NULL)
`, start, end)
	if err != nil {
		return 0, fmt.Errorf("query three-way: %w", err)
	}
	defer rows.Close()

	var mismatches []ReconMismatch
	for rows.Next() {
		var (
			paymentID, merchantID string
			amount                int64
			fee                   int64
			amountRefunded        int64
			settled               bool
			settlementID          *string
			siID                  *string
			siNet                 *int64
		)
		if err := rows.Scan(&paymentID, &merchantID, &amount, &fee, &amountRefunded, &settled, &settlementID, &siID, &siNet); err != nil {
			return 0, err
		}

		expectedNet := settlement.CalculateNet(amount, fee, settlement.CalculateRefundNetImpact(amount, fee, amountRefunded))
		if settled && siID == nil {
			mismatches = append(mismatches, ReconMismatch{
				ID:            idgen.New("mm"),
				BatchID:       batchID,
				MerchantID:    merchantID,
				MismatchType:  MismatchPaymentSettledNotInBatch,
				EntityType:    "payment",
				EntityID:      paymentID,
				ExpectedValue: "settlement_item exists",
				ActualValue:   "no settlement_item found",
				Description:   fmt.Sprintf("payment %s marked settled but no settlement_item", paymentID),
			})
			continue
		}
		if siID != nil && siNet != nil && *siNet != expectedNet {
			mismatches = append(mismatches, ReconMismatch{
				ID:            idgen.New("mm"),
				BatchID:       batchID,
				MerchantID:    merchantID,
				MismatchType:  MismatchSettlementPaymentMismatch,
				EntityType:    "payment",
				EntityID:      paymentID,
				ExpectedValue: fmt.Sprintf("%d", expectedNet),
				ActualValue:   fmt.Sprintf("%d", *siNet),
				Description:   fmt.Sprintf("payment %s settlement net mismatch: expected=%d actual=%d", paymentID, expectedNet, *siNet),
			})
		}
		if !settled && siID != nil {
			mismatches = append(mismatches, ReconMismatch{
				ID:            idgen.New("mm"),
				BatchID:       batchID,
				MerchantID:    merchantID,
				MismatchType:  MismatchSettlementPaymentMismatch,
				EntityType:    "payment",
				EntityID:      paymentID,
				ExpectedValue: "settled=true",
				ActualValue:   "settled=false",
				Description:   fmt.Sprintf("payment %s has settlement_item but is not marked settled", paymentID),
			})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	orphanRows, err := w.db.Query(ctx, `
SELECT si.id, si.merchant_id, si.payment_id
FROM paygate_settlements.settlement_items si
LEFT JOIN paygate_payments.payments p ON p.id = si.payment_id
JOIN paygate_settlements.settlements s ON s.id = si.settlement_id
WHERE s.processed_at >= $1 AND s.processed_at < $2
  AND p.id IS NULL
`, start, end)
	if err != nil {
		return 0, fmt.Errorf("query orphan settlement items: %w", err)
	}
	defer orphanRows.Close()

	for orphanRows.Next() {
		var settlementItemID, merchantID, paymentID string
		if err := orphanRows.Scan(&settlementItemID, &merchantID, &paymentID); err != nil {
			return 0, err
		}
		mismatches = append(mismatches, ReconMismatch{
			ID:            idgen.New("mm"),
			BatchID:       batchID,
			MerchantID:    merchantID,
			MismatchType:  MismatchOrphanSettlementItem,
			EntityType:    "settlement_item",
			EntityID:      settlementItemID,
			ExpectedValue: "payment exists",
			ActualValue:   fmt.Sprintf("missing payment %s", paymentID),
			Description:   fmt.Sprintf("settlement item %s references missing payment %s", settlementItemID, paymentID),
		})
	}
	if err := orphanRows.Err(); err != nil {
		return 0, err
	}

	return w.persistBatch(ctx, batchID, "", BatchTypeThreeWay, start, end, checkedCount, len(mismatches), mismatches)
}

func (w *Worker) countCapturedPayments(ctx context.Context, start, end time.Time) (int, error) {
	var count int
	if err := w.db.QueryRow(ctx, `
SELECT COUNT(*)
FROM paygate_payments.payments
WHERE status = 'captured'
  AND captured_at >= $1 AND captured_at < $2
`, start, end).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// ListMismatches returns recon mismatches for a merchant, newest first.
func (w *Worker) ListMismatches(ctx context.Context, merchantID string, limit int, unresolvedOnly bool) ([]ReconMismatch, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `
SELECT id, batch_id, merchant_id, mismatch_type, entity_type, entity_id,
       expected_value, actual_value, description, resolved, created_at
FROM paygate_recon.recon_mismatches
WHERE merchant_id = $1
  AND ($2 = FALSE OR resolved = FALSE)
ORDER BY created_at DESC
LIMIT $3`

	rows, err := w.db.Query(ctx, q, merchantID, unresolvedOnly, limit)
	if err != nil {
		return nil, fmt.Errorf("list recon mismatches: %w", err)
	}
	defer rows.Close()

	var mismatches []ReconMismatch
	for rows.Next() {
		var mm ReconMismatch
		if err := rows.Scan(
			&mm.ID, &mm.BatchID, &mm.MerchantID, &mm.MismatchType,
			&mm.EntityType, &mm.EntityID, &mm.ExpectedValue, &mm.ActualValue,
			&mm.Description, &mm.Resolved, &mm.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan recon mismatch: %w", err)
		}
		mismatches = append(mismatches, mm)
	}
	return mismatches, rows.Err()
}

func (w *Worker) persistBatch(ctx context.Context, batchID, merchantID string, batchType BatchType, start, end time.Time, checked, mismatchCount int, mismatches []ReconMismatch) (int, error) {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_recon.recon_batches
    (id, merchant_id, batch_type, status, period_start, period_end, checked_count, mismatch_count)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`, batchID, merchantID, batchType, "completed", start, end, checked, mismatchCount); err != nil {
		return 0, fmt.Errorf("insert recon batch: %w", err)
	}

	for _, mm := range mismatches {
		if _, err := tx.Exec(ctx, `
INSERT INTO paygate_recon.recon_mismatches
    (id, batch_id, merchant_id, mismatch_type, entity_type, entity_id, expected_value, actual_value, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`, mm.ID, mm.BatchID, mm.MerchantID, mm.MismatchType, mm.EntityType, mm.EntityID, mm.ExpectedValue, mm.ActualValue, mm.Description); err != nil {
			return 0, fmt.Errorf("insert recon mismatch: %w", err)
		}
		w.logger.Warn("recon mismatch detected",
			"type", mm.MismatchType,
			"entity_type", mm.EntityType,
			"entity_id", mm.EntityID,
			"description", mm.Description,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return mismatchCount, nil
}
