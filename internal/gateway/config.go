package gateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

// ScenarioMode represents a gateway simulation behaviour.
type ScenarioMode string

const (
	ModeSuccess      ScenarioMode = "success"
	ModeSlow         ScenarioMode = "slow"
	ModeFlaky        ScenarioMode = "flaky"
	ModeTimeout      ScenarioMode = "timeout"
	ModeDecline      ScenarioMode = "decline"
	ModeLateCallback ScenarioMode = "late_callback"
)

// Scenario holds the configuration for a single gateway simulation scenario.
type Scenario struct {
	ID          string
	MerchantID  string
	Mode        ScenarioMode
	FailureRate float64
	DelayMS     int
	DeclineCode string
	Active      bool
	CreatedAt   time.Time
}

// ScenarioStore persists and retrieves gateway simulation scenarios.
type ScenarioStore struct {
	db *pgxpool.Pool
}

// NewScenarioStore creates a new ScenarioStore.
func NewScenarioStore(db *pgxpool.Pool) *ScenarioStore {
	return &ScenarioStore{db: db}
}

// Get retrieves the active scenario for a merchant. If no merchant-specific
// scenario is active, it falls back to the global default (merchant_id = '').
// If neither exists, a default success scenario is returned.
func (s *ScenarioStore) Get(ctx context.Context, merchantID string) (Scenario, error) {
	sc, err := s.getActive(ctx, merchantID)
	if err == nil {
		return sc, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Scenario{}, err
	}

	// Fall back to global.
	if merchantID != "" {
		sc, err = s.getActive(ctx, "")
		if err == nil {
			return sc, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Scenario{}, err
		}
	}

	// Return the default success scenario.
	return Scenario{
		Mode:        ModeSuccess,
		FailureRate: 0,
		DelayMS:     0,
		DeclineCode: "CARD_DECLINED",
		Active:      true,
	}, nil
}

func (s *ScenarioStore) getActive(ctx context.Context, merchantID string) (Scenario, error) {
	var sc Scenario
	err := s.db.QueryRow(ctx, `
SELECT id, merchant_id, mode, failure_rate, delay_ms, decline_code, active, created_at
FROM paygate_gateway.gateway_scenarios
WHERE merchant_id = $1 AND active = true
LIMIT 1
`, merchantID).Scan(
		&sc.ID, &sc.MerchantID, &sc.Mode,
		&sc.FailureRate, &sc.DelayMS, &sc.DeclineCode,
		&sc.Active, &sc.CreatedAt,
	)
	return sc, err
}

// Upsert deactivates any existing active scenario for the merchant and inserts
// a new one, returning the newly created scenario.
func (s *ScenarioStore) Upsert(ctx context.Context, sc Scenario) (Scenario, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Scenario{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Deactivate existing active scenario for this merchant.
	if _, err := tx.Exec(ctx, `
UPDATE paygate_gateway.gateway_scenarios
SET active = false, updated_at = NOW()
WHERE merchant_id = $1 AND active = true
`, sc.MerchantID); err != nil {
		return Scenario{}, fmt.Errorf("deactivate existing scenario: %w", err)
	}

	id := idgen.New("gsc")
	if sc.DeclineCode == "" {
		sc.DeclineCode = "CARD_DECLINED"
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_gateway.gateway_scenarios
    (id, merchant_id, mode, failure_rate, delay_ms, decline_code, active)
VALUES ($1, $2, $3, $4, $5, $6, true)
`, id, sc.MerchantID, sc.Mode, sc.FailureRate, sc.DelayMS, sc.DeclineCode); err != nil {
		return Scenario{}, fmt.Errorf("insert scenario: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Scenario{}, err
	}

	var created Scenario
	err = s.db.QueryRow(ctx, `
SELECT id, merchant_id, mode, failure_rate, delay_ms, decline_code, active, created_at
FROM paygate_gateway.gateway_scenarios
WHERE id = $1
`, id).Scan(
		&created.ID, &created.MerchantID, &created.Mode,
		&created.FailureRate, &created.DelayMS, &created.DeclineCode,
		&created.Active, &created.CreatedAt,
	)
	return created, err
}

// List returns all scenarios (up to 50).
func (s *ScenarioStore) List(ctx context.Context) ([]Scenario, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, merchant_id, mode, failure_rate, delay_ms, decline_code, active, created_at
FROM paygate_gateway.gateway_scenarios
ORDER BY created_at DESC
LIMIT 50
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Scenario
	for rows.Next() {
		var sc Scenario
		if err := rows.Scan(
			&sc.ID, &sc.MerchantID, &sc.Mode,
			&sc.FailureRate, &sc.DelayMS, &sc.DeclineCode,
			&sc.Active, &sc.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}
