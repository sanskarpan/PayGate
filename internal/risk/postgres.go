package risk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

func scanRiskEvent(scanner interface {
	Scan(dest ...any) error
}) (RiskEvent, error) {
	var ev RiskEvent
	var rulesJSON []byte
	var resolvedBy *string
	var assignedAt *time.Time
	err := scanner.Scan(
		&ev.ID, &ev.MerchantID, &ev.PaymentID, &ev.Score, &ev.Action, &rulesJSON,
		&ev.DeviceFingerprintHash, &ev.BrowserLanguage, &ev.UserAgent, &ev.CardBIN, &ev.CardNetwork,
		&ev.IssuerCountry, &ev.CardCountry, &ev.FundingType, &ev.ReviewStatus, &ev.AssignedTo, &assignedAt,
		&ev.ReviewNotes, &ev.ManualDecision, &ev.Resolved, &resolvedBy, &ev.ResolvedAt, &ev.CreatedAt, &ev.UpdatedAt,
	)
	if err != nil {
		return RiskEvent{}, err
	}
	ev.AssignedAt = assignedAt
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

func (r *PostgresRepository) CreateRiskEvent(ctx context.Context, ev RiskEvent) (RiskEvent, error) {
	ev.ID = idgen.New("risk")
	rulesJSON, err := json.Marshal(ev.TriggeredRules)
	if err != nil {
		return RiskEvent{}, fmt.Errorf("marshal triggered rules: %w", err)
	}

	q := `
INSERT INTO paygate_risk.risk_events
    (id, merchant_id, payment_id, score, action, triggered_rules, device_fingerprint_hash, browser_language, user_agent,
     card_bin, card_network, issuer_country, card_country, funding_type, review_status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING created_at, updated_at`

	if err := r.db.QueryRow(ctx, q,
		ev.ID, ev.MerchantID, ev.PaymentID, ev.Score, ev.Action, rulesJSON,
		ev.DeviceFingerprintHash, ev.BrowserLanguage, ev.UserAgent, ev.CardBIN, ev.CardNetwork,
		ev.IssuerCountry, ev.CardCountry, ev.FundingType, ev.ReviewStatus,
	).Scan(&ev.CreatedAt, &ev.UpdatedAt); err != nil {
		return RiskEvent{}, fmt.Errorf("insert risk event: %w", err)
	}
	return ev, nil
}

func (r *PostgresRepository) GetRiskEvent(ctx context.Context, merchantID, eventID string) (RiskEvent, error) {
	q := `
SELECT id, merchant_id, payment_id, score, action, triggered_rules,
       device_fingerprint_hash, browser_language, user_agent, card_bin, card_network, issuer_country, card_country,
       funding_type, review_status, assigned_to, assigned_at, review_notes, manual_decision,
       resolved, resolved_by, resolved_at, created_at, updated_at
FROM paygate_risk.risk_events
WHERE merchant_id = $1 AND id = $2`

	ev, err := scanRiskEvent(r.db.QueryRow(ctx, q, merchantID, eventID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RiskEvent{}, ErrRiskEventNotFound
		}
		return RiskEvent{}, fmt.Errorf("get risk event: %w", err)
	}
	return ev, nil
}

func (r *PostgresRepository) ListRiskEvents(ctx context.Context, merchantID string, limit int, unresolvedOnly bool) ([]RiskEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	q := `
SELECT id, merchant_id, payment_id, score, action, triggered_rules,
       device_fingerprint_hash, browser_language, user_agent, card_bin, card_network, issuer_country, card_country,
       funding_type, review_status, assigned_to, assigned_at, review_notes, manual_decision,
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
		ev, err := scanRiskEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan risk event: %w", err)
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

func (r *PostgresRepository) AssignRiskEvent(ctx context.Context, merchantID, eventID, assignedTo string) error {
	cmd, err := r.db.Exec(ctx, `
UPDATE paygate_risk.risk_events
SET assigned_to = $3, assigned_at = NOW(), updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
`, merchantID, eventID, assignedTo)
	if err != nil {
		return fmt.Errorf("assign risk event: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrRiskEventNotFound
	}
	return nil
}

func (r *PostgresRepository) ReviewRiskEvent(ctx context.Context, merchantID, eventID string, status ReviewStatus, notes, actor string) (RiskEvent, error) {
	ev, err := scanRiskEvent(r.db.QueryRow(ctx, `
UPDATE paygate_risk.risk_events
SET review_status = $3,
    review_notes = $4,
    manual_decision = $3,
    resolved = TRUE,
    resolved_by = $5,
    resolved_at = NOW(),
    updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
RETURNING id, merchant_id, payment_id, score, action, triggered_rules,
       device_fingerprint_hash, browser_language, user_agent, card_bin, card_network, issuer_country, card_country,
       funding_type, review_status, assigned_to, assigned_at, review_notes, manual_decision,
       resolved, resolved_by, resolved_at, created_at, updated_at
`, merchantID, eventID, status, notes, actor))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RiskEvent{}, ErrRiskEventNotFound
		}
		return RiskEvent{}, fmt.Errorf("review risk event: %w", err)
	}
	return ev, nil
}

func (r *PostgresRepository) GetMerchantFraudConfig(ctx context.Context, merchantID string) (MerchantFraudConfig, error) {
	cfg := DefaultMerchantFraudConfig(merchantID)
	var blockedCountriesJSON []byte
	var blockedBINsJSON []byte
	err := r.db.QueryRow(ctx, `
SELECT merchant_id, ip_velocity_threshold, device_velocity_threshold, merchant_velocity_threshold, amount_spike_factor,
       review_threshold, block_threshold, blocked_countries, blocked_bins, review_on_country_mismatch, created_at, updated_at
FROM paygate_risk.merchant_fraud_configs
WHERE merchant_id = $1
`, merchantID).Scan(&cfg.MerchantID, &cfg.IPVelocityThreshold, &cfg.DeviceVelocityThreshold, &cfg.MerchantVelocityThreshold, &cfg.AmountSpikeFactor,
		&cfg.ReviewThreshold, &cfg.BlockThreshold, &blockedCountriesJSON, &blockedBINsJSON, &cfg.ReviewOnCountryMismatch, &cfg.CreatedAt, &cfg.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return cfg, nil
		}
		return MerchantFraudConfig{}, fmt.Errorf("get merchant fraud config: %w", err)
	}
	_ = json.Unmarshal(blockedCountriesJSON, &cfg.BlockedCountries)
	_ = json.Unmarshal(blockedBINsJSON, &cfg.BlockedBINs)
	return cfg, nil
}

func (r *PostgresRepository) UpsertMerchantFraudConfig(ctx context.Context, cfg MerchantFraudConfig) (MerchantFraudConfig, error) {
	blockedCountriesJSON, _ := json.Marshal(normalizeUpper(cfg.BlockedCountries))
	blockedBINsJSON, _ := json.Marshal(normalizeUpper(cfg.BlockedBINs))
	err := r.db.QueryRow(ctx, `
INSERT INTO paygate_risk.merchant_fraud_configs
    (merchant_id, ip_velocity_threshold, device_velocity_threshold, merchant_velocity_threshold, amount_spike_factor,
     review_threshold, block_threshold, blocked_countries, blocked_bins, review_on_country_mismatch)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (merchant_id) DO UPDATE
SET ip_velocity_threshold = EXCLUDED.ip_velocity_threshold,
    device_velocity_threshold = EXCLUDED.device_velocity_threshold,
    merchant_velocity_threshold = EXCLUDED.merchant_velocity_threshold,
    amount_spike_factor = EXCLUDED.amount_spike_factor,
    review_threshold = EXCLUDED.review_threshold,
    block_threshold = EXCLUDED.block_threshold,
    blocked_countries = EXCLUDED.blocked_countries,
    blocked_bins = EXCLUDED.blocked_bins,
    review_on_country_mismatch = EXCLUDED.review_on_country_mismatch,
    updated_at = NOW()
RETURNING created_at, updated_at
`, cfg.MerchantID, cfg.IPVelocityThreshold, cfg.DeviceVelocityThreshold, cfg.MerchantVelocityThreshold, cfg.AmountSpikeFactor,
		cfg.ReviewThreshold, cfg.BlockThreshold, blockedCountriesJSON, blockedBINsJSON, cfg.ReviewOnCountryMismatch).
		Scan(&cfg.CreatedAt, &cfg.UpdatedAt)
	if err != nil {
		return MerchantFraudConfig{}, fmt.Errorf("upsert merchant fraud config: %w", err)
	}
	cfg.BlockedCountries = normalizeUpper(cfg.BlockedCountries)
	cfg.BlockedBINs = normalizeUpper(cfg.BlockedBINs)
	return cfg, nil
}

func (r *PostgresRepository) UpsertVelocityCounter(ctx context.Context, dimension, dimValue string, window VelocityWindow, amount int64) (int, error) {
	windowEnd := windowEndTime(window)
	id := idgen.New("vel")

	q := `
INSERT INTO paygate_risk.velocity_counters (id, dimension, dim_value, window_type, count, amount, window_end)
VALUES ($1, $2, $3, $4, 1, $5, $6)
ON CONFLICT (dimension, dim_value, window_type, window_end) DO UPDATE
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
WHERE dimension = $1 AND dim_value = $2 AND window_type = $3 AND window_end = $4
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

func normalizeUpper(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
