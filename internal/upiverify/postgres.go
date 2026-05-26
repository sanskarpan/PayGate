package upiverify

import (
	"context"
	"encoding/json"
	"errors"

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

func (r *PostgresRepository) GetLatest(ctx context.Context, merchantID, vpa string, purpose Purpose) (Verification, error) {
	var item Verification
	var evidence []byte
	err := r.db.QueryRow(ctx, `
SELECT id, merchant_id, vpa, purpose, version, status, provider, provider_reference, evidence_json, verified_at, expires_at, created_at
FROM paygate_billing.vpa_verifications
WHERE merchant_id = $1 AND vpa = $2 AND purpose = $3
ORDER BY version DESC
LIMIT 1
`, merchantID, vpa, purpose).Scan(
		&item.ID, &item.MerchantID, &item.VPA, &item.Purpose, &item.Version, &item.Status, &item.Provider, &item.ProviderReference,
		&evidence, &item.VerifiedAt, &item.ExpiresAt, &item.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Verification{}, ErrVPAVerificationMiss
		}
		return Verification{}, err
	}
	_ = json.Unmarshal(evidence, &item.Evidence)
	return item, nil
}

func (r *PostgresRepository) Record(ctx context.Context, verification Verification) (Verification, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Verification{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var version int
	if err := tx.QueryRow(ctx, `
SELECT COALESCE(MAX(version), 0) + 1
FROM paygate_billing.vpa_verifications
WHERE merchant_id = $1 AND vpa = $2 AND purpose = $3
`, verification.MerchantID, verification.VPA, verification.Purpose).Scan(&version); err != nil {
		return Verification{}, err
	}
	verification.ID = idgen.New("vpaver")
	verification.Version = version
	raw, _ := json.Marshal(verification.Evidence)
	if err := tx.QueryRow(ctx, `
INSERT INTO paygate_billing.vpa_verifications
    (id, merchant_id, vpa, purpose, version, status, provider, provider_reference, evidence_json, verified_at, expires_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING created_at
`, verification.ID, verification.MerchantID, verification.VPA, verification.Purpose, verification.Version, verification.Status, verification.Provider,
		verification.ProviderReference, raw, verification.VerifiedAt, verification.ExpiresAt).Scan(&verification.CreatedAt); err != nil {
		return Verification{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Verification{}, err
	}
	return verification, nil
}
