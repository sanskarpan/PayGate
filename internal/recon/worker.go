package recon

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
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
	var checkedCount int
	for rows.Next() {
		checkedCount++
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

	return w.persistBatch(ctx, batchID, "", BatchTypePaymentLedger, start, end, checkedCount+len(mismatches), len(mismatches), mismatches)
}

// RunThreeWayCheck verifies settled payments have matching settlement_items.
func (w *Worker) RunThreeWayCheck(ctx context.Context, start, end time.Time) (int, error) {
	batchID := idgen.New("recon")

	rows, err := w.db.Query(ctx, `
SELECT p.id, p.merchant_id, p.amount, p.settled, p.settlement_id,
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
	var checkedCount int
	for rows.Next() {
		checkedCount++
		var (
			paymentID, merchantID string
			amount                int64
			settled               bool
			settlementID          *string
			siID                  *string
			siNet                 *int64
		)
		if err := rows.Scan(&paymentID, &merchantID, &amount, &settled, &settlementID, &siID, &siNet); err != nil {
			return 0, err
		}

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
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	return w.persistBatch(ctx, batchID, "", BatchTypeThreeWay, start, end, checkedCount, len(mismatches), mismatches)
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
