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

// PaymentMethod is the payment instrument type used for a transaction.
type PaymentMethod string

const (
	MethodCard        PaymentMethod = "card"
	MethodUPI         PaymentMethod = "upi"
	MethodNetbanking  PaymentMethod = "netbanking"
	MethodWallet      PaymentMethod = "wallet"
)

// MethodProfile holds default simulation parameters for a payment method.
type MethodProfile struct {
	DefaultDelayMS int
	SuccessRate    float64
	DeclineCodes   []string
}

// defaultProfiles maps each supported method to its default simulation profile.
var defaultProfiles = map[PaymentMethod]MethodProfile{
	MethodCard: {
		DefaultDelayMS: 50,
		SuccessRate:    0.97,
		DeclineCodes:   []string{"INSUFFICIENT_FUNDS", "CARD_DECLINED", "EXPIRED_CARD", "STOLEN_CARD"},
	},
	MethodUPI: {
		DefaultDelayMS: 30,
		SuccessRate:    0.95,
		DeclineCodes:   []string{"VPA_INVALID", "UPI_LIMIT_EXCEEDED", "UPI_NOT_REGISTERED"},
	},
	MethodNetbanking: {
		DefaultDelayMS: 150,
		SuccessRate:    0.92,
		DeclineCodes:   []string{"NET_BANKING_FAILED", "BANK_SERVER_ERROR", "SESSION_EXPIRED"},
	},
	MethodWallet: {
		DefaultDelayMS: 20,
		SuccessRate:    0.99,
		DeclineCodes:   []string{"WALLET_INSUFFICIENT_BALANCE", "WALLET_BLOCKED"},
	},
}

// ProfileForMethod returns the default MethodProfile for the given method string.
// Unknown methods fall back to card defaults.
func ProfileForMethod(method string) MethodProfile {
	if p, ok := defaultProfiles[PaymentMethod(method)]; ok {
		return p
	}
	return defaultProfiles[MethodCard]
}

// ErrMethodConfigNotFound is returned when no MethodConfig exists for the lookup key.
var ErrMethodConfigNotFound = errors.New("method config not found")

// MethodConfig holds a per-merchant, per-method gateway simulation override.
type MethodConfig struct {
	ID          string
	MerchantID  string
	Method      PaymentMethod
	Enabled     bool
	SuccessRate float64
	DeclineCode string
	DelayMS     int
	CreatedAt   time.Time
}

// MethodConfigStore persists and retrieves per-merchant method configuration overrides.
type MethodConfigStore struct {
	db *pgxpool.Pool
}

// NewMethodConfigStore creates a new MethodConfigStore.
func NewMethodConfigStore(db *pgxpool.Pool) *MethodConfigStore {
	return &MethodConfigStore{db: db}
}

// Upsert inserts or updates a MethodConfig row identified by (merchant_id, method).
func (s *MethodConfigStore) Upsert(ctx context.Context, cfg MethodConfig) (MethodConfig, error) {
	if cfg.ID == "" {
		cfg.ID = idgen.New("gmc")
	}
	_, err := s.db.Exec(ctx, `
INSERT INTO paygate_gateway.method_configs
    (id, merchant_id, method, enabled, success_rate, decline_code, delay_ms)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (merchant_id, method) DO UPDATE
    SET enabled      = EXCLUDED.enabled,
        success_rate = EXCLUDED.success_rate,
        decline_code = EXCLUDED.decline_code,
        delay_ms     = EXCLUDED.delay_ms,
        updated_at   = NOW()
`, cfg.ID, cfg.MerchantID, cfg.Method, cfg.Enabled, cfg.SuccessRate, cfg.DeclineCode, cfg.DelayMS)
	if err != nil {
		return MethodConfig{}, fmt.Errorf("upsert method config: %w", err)
	}

	return s.Get(ctx, cfg.MerchantID, string(cfg.Method))
}

// Get retrieves the MethodConfig for the given merchant and method.
// Returns ErrMethodConfigNotFound when no row exists.
func (s *MethodConfigStore) Get(ctx context.Context, merchantID, method string) (MethodConfig, error) {
	var cfg MethodConfig
	err := s.db.QueryRow(ctx, `
SELECT id, merchant_id, method, enabled, success_rate, decline_code, delay_ms, created_at
FROM paygate_gateway.method_configs
WHERE merchant_id = $1 AND method = $2
`, merchantID, method).Scan(
		&cfg.ID, &cfg.MerchantID, &cfg.Method,
		&cfg.Enabled, &cfg.SuccessRate, &cfg.DeclineCode,
		&cfg.DelayMS, &cfg.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return MethodConfig{}, ErrMethodConfigNotFound
	}
	if err != nil {
		return MethodConfig{}, fmt.Errorf("get method config: %w", err)
	}
	return cfg, nil
}

// List returns all MethodConfig rows (up to 200).
func (s *MethodConfigStore) List(ctx context.Context) ([]MethodConfig, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, merchant_id, method, enabled, success_rate, decline_code, delay_ms, created_at
FROM paygate_gateway.method_configs
ORDER BY created_at DESC
LIMIT 200
`)
	if err != nil {
		return nil, fmt.Errorf("list method configs: %w", err)
	}
	defer rows.Close()

	var out []MethodConfig
	for rows.Next() {
		var cfg MethodConfig
		if err := rows.Scan(
			&cfg.ID, &cfg.MerchantID, &cfg.Method,
			&cfg.Enabled, &cfg.SuccessRate, &cfg.DeclineCode,
			&cfg.DelayMS, &cfg.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan method config: %w", err)
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}
