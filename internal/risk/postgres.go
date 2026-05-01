package risk

import (
	"context"
	"encoding/json"
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

func (r *PostgresRepository) CreateRiskEvent(ctx context.Context, ev RiskEvent) (RiskEvent, error) {
	ev.ID = idgen.New("risk")
	rulesJSON, err := json.Marshal(ev.TriggeredRules)
	if err != nil {
		return RiskEvent{}, fmt.Errorf("marshal triggered rules: %w", err)
	}

	q := `
INSERT INTO paygate_risk.risk_events
    (id, merchant_id, payment_id, score, action, triggered_rules)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING created_at, updated_at`

	if err := r.db.QueryRow(ctx, q,
		ev.ID, ev.MerchantID, ev.PaymentID, ev.Score, ev.Action, rulesJSON,
	).Scan(&ev.CreatedAt, &ev.UpdatedAt); err != nil {
		return RiskEvent{}, fmt.Errorf("insert risk event: %w", err)
	}
	return ev, nil
}

func (r *PostgresRepository) GetRiskEvent(ctx context.Context, merchantID, eventID string) (RiskEvent, error) {
	q := `
SELECT id, merchant_id, payment_id, score, action, triggered_rules,
       resolved, resolved_by, resolved_at, created_at, updated_at
FROM paygate_risk.risk_events
WHERE merchant_id = $1 AND id = $2`

	var ev RiskEvent
	var rulesJSON []byte
	var resolvedBy *string
	err := r.db.QueryRow(ctx, q, merchantID, eventID).Scan(
		&ev.ID, &ev.MerchantID, &ev.PaymentID, &ev.Score, &ev.Action, &rulesJSON,
		&ev.Resolved, &resolvedBy, &ev.ResolvedAt, &ev.CreatedAt, &ev.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RiskEvent{}, ErrRiskEventNotFound
		}
		return RiskEvent{}, fmt.Errorf("get risk event: %w", err)
	}
	if resolvedBy != nil {
		ev.ResolvedBy = *resolvedBy
	}
	if len(rulesJSON) > 0 {
		if err := json.Unmarshal(rulesJSON, &ev.TriggeredRules); err != nil {
			return RiskEvent{}, fmt.Errorf("unmarshal rules: %w", err)
		}
	}
	return ev, nil
}

func (r *PostgresRepository) ListRiskEvents(ctx context.Context, merchantID string, limit int, unresolvedOnly bool) ([]RiskEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	q := `
SELECT id, merchant_id, payment_id, score, action, triggered_rules,
       resolved, resolved_by, resolved_at, created_at, updated_at
FROM paygate_risk.risk_events
WHERE merchant_id = $1
  AND ($2 = FALSE OR resolved = FALSE)
ORDER BY created_at DESC
LIMIT $3`

	rows, err := r.db.Query(ctx, q, merchantID, unresolvedOnly, limit)
	if err != nil {
		return nil, fmt.Errorf("list risk events: %w", err)
	}
	defer rows.Close()

	var events []RiskEvent
	for rows.Next() {
		var ev RiskEvent
		var rulesJSON []byte
		var resolvedBy *string
		if err := rows.Scan(
			&ev.ID, &ev.MerchantID, &ev.PaymentID, &ev.Score, &ev.Action, &rulesJSON,
			&ev.Resolved, &resolvedBy, &ev.ResolvedAt, &ev.CreatedAt, &ev.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan risk event: %w", err)
		}
		if resolvedBy != nil {
			ev.ResolvedBy = *resolvedBy
		}
		if len(rulesJSON) > 0 {
			if err := json.Unmarshal(rulesJSON, &ev.TriggeredRules); err != nil {
				return nil, fmt.Errorf("unmarshal rules: %w", err)
			}
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

func (r *PostgresRepository) ResolveRiskEvent(ctx context.Context, merchantID, eventID, resolvedBy string) error {
	cmd, err := r.db.Exec(ctx, `
UPDATE paygate_risk.risk_events
SET resolved = TRUE, resolved_by = $3, resolved_at = NOW(), updated_at = NOW()
WHERE merchant_id = $1 AND id = $2 AND resolved = FALSE`, merchantID, eventID, resolvedBy)
	if err != nil {
		return fmt.Errorf("resolve risk event: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrRiskEventNotFound
	}
	return nil
}

func (r *PostgresRepository) UpsertVelocityCounter(ctx context.Context, dimension, dimValue string, window VelocityWindow, amount int64) (int, error) {
	windowEnd := windowEndTime(window)
	id := idgen.New("vel")

	q := `
INSERT INTO paygate_risk.velocity_counters (id, dimension, dim_value, window, count, amount, window_end)
VALUES ($1, $2, $3, $4, 1, $5, $6)
ON CONFLICT (dimension, dim_value, window, window_end) DO UPDATE
SET count = paygate_risk.velocity_counters.count + 1,
    amount = paygate_risk.velocity_counters.amount + EXCLUDED.amount,
    updated_at = NOW()
RETURNING count`

	var count int
	if err := r.db.QueryRow(ctx, q, id, dimension, dimValue, window, amount, windowEnd).Scan(&count); err != nil {
		return 0, fmt.Errorf("upsert velocity counter: %w", err)
	}
	return count, nil
}

func (r *PostgresRepository) GetVelocityCount(ctx context.Context, dimension, dimValue string, window VelocityWindow) (int, error) {
	windowEnd := windowEndTime(window)
	var count int
	err := r.db.QueryRow(ctx, `
SELECT COALESCE(count, 0)
FROM paygate_risk.velocity_counters
WHERE dimension = $1 AND dim_value = $2 AND window = $3 AND window_end = $4
`, dimension, dimValue, window, windowEnd).Scan(&count)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get velocity count: %w", err)
	}
	return count, nil
}

func (r *PostgresRepository) MerchantAverageTxnAmount(ctx context.Context, merchantID string) (int64, error) {
	var avg int64
	err := r.db.QueryRow(ctx, `
SELECT COALESCE(AVG(amount)::BIGINT, 0)
FROM paygate_payments.payments
WHERE merchant_id = $1
  AND status = 'captured'
  AND created_at > NOW() - INTERVAL '30 days'
`, merchantID).Scan(&avg)
	if err != nil {
		return 0, fmt.Errorf("merchant average txn amount: %w", err)
	}
	return avg, nil
}

// windowEndTime returns the bucket boundary for a rolling window.
// Buckets are aligned to the hour (1h) or day (24h) boundary.
func windowEndTime(w VelocityWindow) time.Time {
	now := time.Now().UTC()
	switch w {
	case VelocityWindow24H:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
	default: // 1h
		return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC).Add(time.Hour)
	}
}
