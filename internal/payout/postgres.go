package payout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
func (r *PostgresRepository) CreateForSettlement(ctx context.Context, merchantID, settlementID, beneficiaryID string, amount int64, currency string, approvalStatus ApprovalStatus, batchID string) (Payout, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Payout{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id := idgen.New("payout")
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_payouts.payouts
    (id, merchant_id, settlement_id, beneficiary_id, approval_status, batch_id, status, amount, currency)
VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), 'pending', $7, $8)
`, id, merchantID, settlementID, beneficiaryID, approvalStatus, batchID, amount, currency); err != nil {
		return Payout{}, fmt.Errorf("insert payout: %w", err)
	}
	if err := r.recordEventTx(ctx, tx, TimelineEvent{
		ID:           idgen.New("poevt"),
		PayoutID:     id,
		MerchantID:   merchantID,
		EventType:    "payout.created",
		StatusBefore: StatePending,
		StatusAfter:  StatePending,
		Payload: map[string]any{
			"settlement_id":   settlementID,
			"beneficiary_id":  beneficiaryID,
			"amount":          amount,
			"currency":        currency,
			"approval_status": approvalStatus,
		},
	}); err != nil {
		return Payout{}, fmt.Errorf("record payout.created event: %w", err)
	}

	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "payout",
		AggregateID:   id,
		EventType:     "payout.created",
		MerchantID:    merchantID,
		Payload: map[string]any{
			"payout_id":       id,
			"settlement_id":   settlementID,
			"beneficiary_id":  beneficiaryID,
			"amount":          amount,
			"currency":        currency,
			"approval_status": approvalStatus,
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
	row := r.db.QueryRow(ctx, selectPayoutSQL+` WHERE id = $1 AND merchant_id = $2`, id, merchantID)
	p, err := scanPayout(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payout{}, ErrPayoutNotFound
	}
	return p, err
}

// GetBySettlementID returns the payout for a settlement scoped to the merchant.
func (r *PostgresRepository) GetBySettlementID(ctx context.Context, merchantID, settlementID string) (Payout, error) {
	row := r.db.QueryRow(ctx, selectPayoutSQL+` WHERE settlement_id = $1 AND merchant_id = $2 LIMIT 1`, settlementID, merchantID)
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
	rows, err := r.db.Query(ctx, selectPayoutSQL+` WHERE merchant_id = $1 ORDER BY created_at DESC LIMIT $2`, merchantID, limit)
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

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payouts.payouts
SET status = 'processing', initiated_at = NOW(), updated_at = NOW()
WHERE id = $1
`, id); err != nil {
		return Payout{}, fmt.Errorf("update payout initiate: %w", err)
	}
	if err := r.recordEventTx(ctx, tx, TimelineEvent{
		ID:           idgen.New("poevt"),
		PayoutID:     id,
		MerchantID:   merchantID,
		EventType:    "payout.initiated",
		StatusBefore: current,
		StatusAfter:  StateProcessing,
		Payload:      map[string]any{"payout_id": id, "occurred_at": now.Unix()},
	}); err != nil {
		return Payout{}, fmt.Errorf("record payout.initiated event: %w", err)
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

	current, err := r.getPayoutForUpdate(ctx, tx, merchantID, id)
	if err != nil {
		return Payout{}, err
	}
	if _, err := Transition(current.Status, EventComplete); err != nil {
		return Payout{}, err
	}
	if err := r.completePayoutTx(ctx, tx, current, bankReference, time.Now().UTC()); err != nil {
		return Payout{}, err
	}
	if err := r.recordEventTx(ctx, tx, TimelineEvent{
		ID:           idgen.New("poevt"),
		PayoutID:     id,
		MerchantID:   merchantID,
		EventType:    "payout.completed",
		StatusBefore: current.Status,
		StatusAfter:  StateCompleted,
		Payload: map[string]any{
			"payout_id":      id,
			"bank_reference": bankReference,
			"amount":         current.Amount,
			"currency":       current.Currency,
		},
	}); err != nil {
		return Payout{}, fmt.Errorf("record payout.completed event: %w", err)
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

	current, err := r.getPayoutForUpdate(ctx, tx, merchantID, id)
	if err != nil {
		return Payout{}, err
	}
	if _, err := Transition(current.Status, EventFail); err != nil {
		return Payout{}, err
	}
	if err := r.failPayoutTx(ctx, tx, current, reason, time.Now().UTC()); err != nil {
		return Payout{}, err
	}
	if err := r.recordEventTx(ctx, tx, TimelineEvent{
		ID:           idgen.New("poevt"),
		PayoutID:     id,
		MerchantID:   merchantID,
		EventType:    "payout.failed",
		StatusBefore: current.Status,
		StatusAfter:  StateFailed,
		Payload:      map[string]any{"payout_id": id, "reason": reason},
	}); err != nil {
		return Payout{}, fmt.Errorf("record payout.failed event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Payout{}, err
	}
	return r.GetByID(ctx, merchantID, id)
}

func (r *PostgresRepository) Cancel(ctx context.Context, merchantID, id, reason string) (Payout, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Payout{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := r.getPayoutForUpdate(ctx, tx, merchantID, id)
	if err != nil {
		return Payout{}, err
	}
	switch current.Status {
	case StateCancelled:
		return current, nil
	case StatePending, StateProcessing:
	default:
		return Payout{}, ErrInvalidTransition
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payouts.payouts
SET status = 'cancelled',
    cancel_reason = NULLIF($2, ''),
    cancelled_at = $3,
    updated_at = $3
WHERE id = $1
`, id, reason, now); err != nil {
		return Payout{}, err
	}
	if err := r.recordEventTx(ctx, tx, TimelineEvent{
		ID:           idgen.New("poevt"),
		PayoutID:     id,
		MerchantID:   merchantID,
		EventType:    "payout.cancelled",
		StatusBefore: current.Status,
		StatusAfter:  StateCancelled,
		Payload:      map[string]any{"payout_id": id, "reason": reason},
	}); err != nil {
		return Payout{}, fmt.Errorf("record payout.cancelled event: %w", err)
	}
	if err := r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "payout",
		AggregateID:   id,
		EventType:     "payout.cancelled",
		MerchantID:    merchantID,
		Payload:       map[string]any{"payout_id": id, "reason": reason},
	}); err != nil {
		return Payout{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Payout{}, err
	}
	return r.GetByID(ctx, merchantID, id)
}

func (r *PostgresRepository) ApplyRailCallback(ctx context.Context, callback RailCallback, signature string) (Payout, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Payout{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := r.getPayoutForUpdate(ctx, tx, callback.MerchantID, callback.PayoutID)
	if err != nil {
		return Payout{}, false, err
	}

	payloadHash := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%s", callback.PayoutID, callback.MerchantID, callback.Status, callback.RailReference, callback.Reason)))
	tag, err := tx.Exec(ctx, `
INSERT INTO paygate_payouts.payout_callback_receipts
    (id, callback_event_id, payout_id, merchant_id, callback_status, signature, payload_hash)
VALUES
    ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (callback_event_id) DO NOTHING
`, idgen.New("porcpt"), callback.EventID, callback.PayoutID, callback.MerchantID, callback.Status, signature, hex.EncodeToString(payloadHash[:]))
	if err != nil {
		return Payout{}, false, err
	}
	if tag.RowsAffected() == 0 {
		return current, false, nil
	}

	updated, err := r.applyRailCallbackTx(ctx, tx, current, callback)
	if err != nil {
		return Payout{}, true, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Payout{}, true, err
	}
	return updated, true, nil
}

func (r *PostgresRepository) ListEvents(ctx context.Context, merchantID, payoutID string, limit int) ([]TimelineEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
SELECT id, payout_id, merchant_id, event_type, status_before, status_after, callback_event_id, payload_json, created_at
FROM paygate_payouts.payout_events
WHERE payout_id = $1 AND merchant_id = $2
ORDER BY created_at DESC
LIMIT $3
`, payoutID, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TimelineEvent
	for rows.Next() {
		item, err := scanTimelineEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) AttachSaga(ctx context.Context, merchantID, id, sagaID string) (Payout, error) {
	if _, err := r.db.Exec(ctx, `
UPDATE paygate_payouts.payouts
SET saga_id = $3, updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
`, id, merchantID, sagaID); err != nil {
		return Payout{}, fmt.Errorf("attach payout saga: %w", err)
	}
	return r.GetByID(ctx, merchantID, id)
}

func (r *PostgresRepository) UpsertSimulatorScenario(ctx context.Context, merchantID, settlementID string, scenario SimulatorScenario) (SimulatorScenario, error) {
	body, err := json.Marshal(map[string]any{
		"steps": scenario.Steps,
	})
	if err != nil {
		return SimulatorScenario{}, err
	}
	row := r.db.QueryRow(ctx, `
INSERT INTO paygate_payouts.payout_simulator_scenarios
    (id, merchant_id, settlement_id, transient_failures_remaining, scenario_json, notes)
VALUES
    ($1, $2, $3, $4, $5, $6)
ON CONFLICT (merchant_id, settlement_id)
DO UPDATE SET transient_failures_remaining = EXCLUDED.transient_failures_remaining,
              scenario_json = EXCLUDED.scenario_json,
              notes = EXCLUDED.notes,
              updated_at = NOW()
RETURNING id, merchant_id, settlement_id, transient_failures_remaining, scenario_json, notes, created_at, updated_at
`, idgen.New("pscn"), merchantID, settlementID, scenario.TransientFailuresRemaining, body, scenario.Notes)
	return scanSimulatorScenario(row)
}

func (r *PostgresRepository) GetSimulatorScenario(ctx context.Context, merchantID, settlementID string) (SimulatorScenario, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, merchant_id, settlement_id, transient_failures_remaining, scenario_json, notes, created_at, updated_at
FROM paygate_payouts.payout_simulator_scenarios
WHERE merchant_id = $1 AND settlement_id = $2
`, merchantID, settlementID)
	item, err := scanSimulatorScenario(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return SimulatorScenario{}, ErrSimulatorScenarioNotFound
	}
	return item, err
}

func (r *PostgresRepository) GetSimulatorScenarioForPayout(ctx context.Context, merchantID, payoutID string) (SimulatorScenario, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return SimulatorScenario{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var scenario SimulatorScenario
	row := tx.QueryRow(ctx, `
SELECT s.id, s.merchant_id, s.settlement_id, s.transient_failures_remaining, s.scenario_json, s.notes, s.created_at, s.updated_at
FROM paygate_payouts.payout_simulator_scenarios s
JOIN paygate_payouts.payouts p ON p.settlement_id = s.settlement_id AND p.merchant_id = s.merchant_id
WHERE p.id = $1 AND p.merchant_id = $2
FOR UPDATE
`, payoutID, merchantID)
	scenario, err = scanSimulatorScenario(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return SimulatorScenario{}, false, nil
	}
	if err != nil {
		return SimulatorScenario{}, false, err
	}
	shouldFail := scenario.TransientFailuresRemaining > 0
	if shouldFail {
		if _, err := tx.Exec(ctx, `
UPDATE paygate_payouts.payout_simulator_scenarios
SET transient_failures_remaining = transient_failures_remaining - 1,
    updated_at = NOW()
WHERE id = $1
`, scenario.ID); err != nil {
			return SimulatorScenario{}, false, err
		}
		scenario.TransientFailuresRemaining--
	}
	if err := tx.Commit(ctx); err != nil {
		return SimulatorScenario{}, false, err
	}
	return scenario, shouldFail, nil
}

func (r *PostgresRepository) getPayoutForUpdate(ctx context.Context, tx pgx.Tx, merchantID, id string) (Payout, error) {
	row := tx.QueryRow(ctx, selectPayoutSQL+` WHERE id = $1 AND merchant_id = $2 FOR UPDATE`, id, merchantID)
	current, err := scanPayout(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payout{}, ErrPayoutNotFound
	}
	return current, err
}

func (r *PostgresRepository) applyRailCallbackTx(ctx context.Context, tx pgx.Tx, current Payout, callback RailCallback) (Payout, error) {
	before := current.Status
	after := before
	eventType := eventTypeForRailStatus(callback.Status)
	now := callback.OccurredAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	payload := map[string]any{
		"event_id":       callback.EventID,
		"payout_id":      callback.PayoutID,
		"merchant_id":    callback.MerchantID,
		"status":         callback.Status,
		"rail_reference": callback.RailReference,
		"reason":         callback.Reason,
		"occurred_at":    now.Unix(),
	}

	switch callback.Status {
	case RailStatusProcessing:
		if current.Status == StatePending {
			if _, err := tx.Exec(ctx, `
UPDATE paygate_payouts.payouts
SET status = 'processing',
    initiated_at = COALESCE(initiated_at, $3),
    rail_reference = NULLIF($2, ''),
    updated_at = $3
WHERE id = $1
`, current.ID, callback.RailReference, now); err != nil {
				return Payout{}, err
			}
			after = StateProcessing
		}
	case RailStatusCompleted:
		switch current.Status {
		case StateProcessing:
			if err := r.completePayoutTx(ctx, tx, current, callback.RailReference, now); err != nil {
				return Payout{}, err
			}
			after = StateCompleted
		case StateCompleted, StateReturned, StateReversed:
			after = current.Status
		}
	case RailStatusFailed:
		if current.Status == StateProcessing {
			if err := r.failPayoutTx(ctx, tx, current, callback.Reason, now); err != nil {
				return Payout{}, err
			}
			after = StateFailed
		}
	case RailStatusReturned:
		switch current.Status {
		case StateProcessing:
			if err := r.markReturnedWithoutLedgerTx(ctx, tx, current, callback.RailReference, callback.Reason, now, StateReturned); err != nil {
				return Payout{}, err
			}
			after = StateReturned
		case StateCompleted:
			if err := r.reversePayoutLedgerTx(ctx, tx, current, "payout_return", "payout.returned", callback.RailReference, callback.Reason, now, StateReturned); err != nil {
				return Payout{}, err
			}
			after = StateReturned
		}
	case RailStatusReversed:
		switch current.Status {
		case StateProcessing:
			if err := r.markReturnedWithoutLedgerTx(ctx, tx, current, callback.RailReference, callback.Reason, now, StateReversed); err != nil {
				return Payout{}, err
			}
			after = StateReversed
		case StateCompleted:
			if err := r.reversePayoutLedgerTx(ctx, tx, current, "payout_reversal", "payout.reversed", callback.RailReference, callback.Reason, now, StateReversed); err != nil {
				return Payout{}, err
			}
			after = StateReversed
		}
	}

	if err := r.recordEventTx(ctx, tx, TimelineEvent{
		ID:              idgen.New("poevt"),
		PayoutID:        current.ID,
		MerchantID:      current.MerchantID,
		EventType:       eventType,
		StatusBefore:    before,
		StatusAfter:     after,
		CallbackEventID: callback.EventID,
		Payload:         payload,
	}); err != nil {
		return Payout{}, err
	}
	row := tx.QueryRow(ctx, selectPayoutSQL+` WHERE id = $1 AND merchant_id = $2`, current.ID, current.MerchantID)
	return scanPayout(row)
}

func (r *PostgresRepository) completePayoutTx(ctx context.Context, tx pgx.Tx, current Payout, railReference string, now time.Time) error {
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payouts.payouts
SET status = 'completed',
    bank_reference = $2,
    rail_reference = $2,
    completed_at = $3,
    updated_at = $3
WHERE id = $1
`, current.ID, railReference, now); err != nil {
		return fmt.Errorf("update payout complete: %w", err)
	}
	if _, err := r.ledger.CreateEntriesTx(ctx, tx, current.MerchantID, "payout", current.ID,
		fmt.Sprintf("payout %s completed", current.ID),
		[]ledger.Entry{
			{AccountCode: "SETTLEMENT_CLEARING", DebitAmount: current.Amount, Currency: current.Currency, Description: "settlement clearing debit on payout"},
			{AccountCode: "MERCHANT_BANK_PAYOUT", CreditAmount: current.Amount, Currency: current.Currency, Description: "merchant bank payout credit"},
		},
	); err != nil {
		return fmt.Errorf("write payout ledger entries: %w", err)
	}
	return r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "payout",
		AggregateID:   current.ID,
		EventType:     "payout.completed",
		MerchantID:    current.MerchantID,
		Payload: map[string]any{
			"payout_id":      current.ID,
			"bank_reference": railReference,
			"amount":         current.Amount,
			"currency":       current.Currency,
		},
	})
}

func (r *PostgresRepository) failPayoutTx(ctx context.Context, tx pgx.Tx, current Payout, reason string, now time.Time) error {
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payouts.payouts
SET status = 'failed', failure_reason = $2, failed_at = $3, updated_at = $3
WHERE id = $1
`, current.ID, reason, now); err != nil {
		return fmt.Errorf("update payout fail: %w", err)
	}
	return r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "payout",
		AggregateID:   current.ID,
		EventType:     "payout.failed",
		MerchantID:    current.MerchantID,
		Payload:       map[string]any{"payout_id": current.ID, "reason": reason},
	})
}

func (r *PostgresRepository) markReturnedWithoutLedgerTx(ctx context.Context, tx pgx.Tx, current Payout, railReference, reason string, now time.Time, targetState PayoutState) error {
	column := "returned_at"
	if targetState == StateReversed {
		column = "reversed_at"
	}
	query := fmt.Sprintf(`
UPDATE paygate_payouts.payouts
SET status = $2,
    rail_reference = NULLIF($3, ''),
    return_reason = $4,
    %s = $5,
    updated_at = $5
WHERE id = $1
`, column)
	if _, err := tx.Exec(ctx, query, current.ID, targetState, railReference, reason, now); err != nil {
		return err
	}
	return r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "payout",
		AggregateID:   current.ID,
		EventType:     fmt.Sprintf("payout.%s", targetState),
		MerchantID:    current.MerchantID,
		Payload: map[string]any{
			"payout_id":      current.ID,
			"rail_reference": railReference,
			"reason":         reason,
		},
	})
}

func (r *PostgresRepository) reversePayoutLedgerTx(ctx context.Context, tx pgx.Tx, current Payout, sourceType, eventType, railReference, reason string, now time.Time, targetState PayoutState) error {
	column := "returned_at"
	if targetState == StateReversed {
		column = "reversed_at"
	}
	query := fmt.Sprintf(`
UPDATE paygate_payouts.payouts
SET status = $2,
    rail_reference = NULLIF($3, ''),
    return_reason = $4,
    %s = $5,
    updated_at = $5
WHERE id = $1
`, column)
	if _, err := tx.Exec(ctx, query, current.ID, targetState, railReference, reason, now); err != nil {
		return err
	}
	if _, err := r.ledger.CreateEntriesTx(ctx, tx, current.MerchantID, sourceType, current.ID,
		fmt.Sprintf("%s for payout %s", eventType, current.ID),
		[]ledger.Entry{
			{AccountCode: "MERCHANT_BANK_PAYOUT", DebitAmount: current.Amount, Currency: current.Currency, Description: fmt.Sprintf("%s debit", eventType)},
			{AccountCode: "SETTLEMENT_CLEARING", CreditAmount: current.Amount, Currency: current.Currency, Description: fmt.Sprintf("%s credit", eventType)},
		},
	); err != nil {
		return fmt.Errorf("write payout corrective ledger entries: %w", err)
	}
	return r.outbox.WriteTx(ctx, tx, outbox.Event{
		AggregateType: "payout",
		AggregateID:   current.ID,
		EventType:     eventType,
		MerchantID:    current.MerchantID,
		Payload: map[string]any{
			"payout_id":      current.ID,
			"rail_reference": railReference,
			"reason":         reason,
			"amount":         current.Amount,
			"currency":       current.Currency,
		},
	})
}

func (r *PostgresRepository) recordEventTx(ctx context.Context, tx pgx.Tx, event TimelineEvent) error {
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	body, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO paygate_payouts.payout_events
    (id, payout_id, merchant_id, event_type, status_before, status_after, callback_event_id, payload_json)
VALUES
    ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8)
`, event.ID, event.PayoutID, event.MerchantID, event.EventType, event.StatusBefore, event.StatusAfter, event.CallbackEventID, body)
	return err
}

const selectPayoutSQL = `
SELECT id, merchant_id, settlement_id, beneficiary_id, status, approval_status, amount, currency,
       saga_id, bank_reference, rail_reference, failure_reason, return_reason, cancel_reason,
       initiated_at, completed_at, failed_at, returned_at, reversed_at, cancelled_at, created_at, updated_at
FROM paygate_payouts.payouts`

// scanPayout is a helper that scans a payout row from any pgx row-like interface.
type scannable interface {
	Scan(dest ...any) error
}

func scanPayout(row scannable) (Payout, error) {
	var p Payout
	var beneficiaryID *string
	var approvalStatus *string
	var sagaID *string
	var bankRef *string
	var railRef *string
	var failureReason *string
	var returnReason *string
	var cancelReason *string
	err := row.Scan(
		&p.ID, &p.MerchantID, &p.SettlementID, &beneficiaryID, &p.Status, &approvalStatus,
		&p.Amount, &p.Currency,
		&sagaID, &bankRef, &railRef, &failureReason, &returnReason, &cancelReason,
		&p.InitiatedAt, &p.CompletedAt, &p.FailedAt, &p.ReturnedAt, &p.ReversedAt, &p.CancelledAt,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Payout{}, err
	}
	if beneficiaryID != nil {
		p.BeneficiaryID = *beneficiaryID
	}
	if approvalStatus != nil {
		p.ApprovalStatus = ApprovalStatus(*approvalStatus)
	}
	if sagaID != nil {
		p.SagaID = *sagaID
	}
	if bankRef != nil {
		p.BankReference = *bankRef
	}
	if railRef != nil {
		p.RailReference = *railRef
	}
	if failureReason != nil {
		p.FailureReason = *failureReason
	}
	if returnReason != nil {
		p.ReturnReason = *returnReason
	}
	if cancelReason != nil {
		p.CancelReason = *cancelReason
	}
	return p, nil
}

func scanTimelineEvent(row scannable) (TimelineEvent, error) {
	var event TimelineEvent
	var callbackEventID *string
	var payloadJSON []byte
	err := row.Scan(&event.ID, &event.PayoutID, &event.MerchantID, &event.EventType, &event.StatusBefore, &event.StatusAfter, &callbackEventID, &payloadJSON, &event.CreatedAt)
	if err != nil {
		return TimelineEvent{}, err
	}
	if callbackEventID != nil {
		event.CallbackEventID = *callbackEventID
	}
	if err := json.Unmarshal(payloadJSON, &event.Payload); err != nil {
		return TimelineEvent{}, err
	}
	return event, nil
}

func scanSimulatorScenario(row scannable) (SimulatorScenario, error) {
	var item SimulatorScenario
	var scenarioJSON []byte
	err := row.Scan(&item.ID, &item.MerchantID, &item.SettlementID, &item.TransientFailuresRemaining, &scenarioJSON, &item.Notes, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return SimulatorScenario{}, err
	}
	var payload struct {
		Steps []SimulatorScenarioStep `json:"steps"`
	}
	if err := json.Unmarshal(scenarioJSON, &payload); err != nil {
		return SimulatorScenario{}, err
	}
	item.Steps = payload.Steps
	return item, nil
}
