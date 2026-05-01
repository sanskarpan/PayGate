package merchant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateMerchant(ctx context.Context, merchant Merchant) (Merchant, error) {
	settings := merchant.Settings
	if settings == nil {
		settings = map[string]any{}
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return Merchant{}, fmt.Errorf("marshal merchant settings: %w", err)
	}

	q := `
INSERT INTO paygate_merchants.merchants (id, name, email, business_type, status, settings)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING created_at, updated_at`

	row := r.db.QueryRow(ctx, q, merchant.ID, merchant.Name, merchant.Email, merchant.BusinessType, merchant.Status, settingsJSON)
	if err := row.Scan(&merchant.CreatedAt, &merchant.UpdatedAt); err != nil {
		return Merchant{}, fmt.Errorf("insert merchant: %w", err)
	}

	merchant.Settings = settings
	return merchant, nil
}

func (r *PostgresRepository) GetMerchantByID(ctx context.Context, merchantID string) (Merchant, error) {
	q := `
SELECT id, name, email, business_type, status, settings, created_at, updated_at
FROM paygate_merchants.merchants
WHERE id = $1`

	var m Merchant
	var rawSettings []byte
	if err := r.db.QueryRow(ctx, q, merchantID).Scan(
		&m.ID,
		&m.Name,
		&m.Email,
		&m.BusinessType,
		&m.Status,
		&rawSettings,
		&m.CreatedAt,
		&m.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Merchant{}, ErrMerchantNotFound
		}
		return Merchant{}, fmt.Errorf("get merchant by id: %w", err)
	}

	if len(rawSettings) > 0 {
		if err := json.Unmarshal(rawSettings, &m.Settings); err != nil {
			return Merchant{}, fmt.Errorf("unmarshal merchant settings: %w", err)
		}
	}
	if m.Settings == nil {
		m.Settings = map[string]any{}
	}

	return m, nil
}

func (r *PostgresRepository) CreateAPIKey(ctx context.Context, key APIKey) (APIKey, error) {
	q := `
INSERT INTO paygate_merchants.api_keys (id, merchant_id, secret_hash, mode, scope, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING created_at`

	if err := r.db.QueryRow(ctx, q, key.ID, key.MerchantID, key.SecretHash, key.Mode, key.Scope, key.Status).Scan(&key.CreatedAt); err != nil {
		return APIKey{}, fmt.Errorf("insert api key: %w", err)
	}

	return key, nil
}

func (r *PostgresRepository) ListAPIKeysByMerchant(ctx context.Context, merchantID string) ([]APIKey, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, secret_hash, mode, scope, status, allowed_ips, last_used_at, revoked_at, created_at
FROM paygate_merchants.api_keys
WHERE merchant_id = $1
ORDER BY created_at DESC
`, merchantID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var key APIKey
		if err := rows.Scan(
			&key.ID,
			&key.MerchantID,
			&key.SecretHash,
			&key.Mode,
			&key.Scope,
			&key.Status,
			&key.AllowedIPs,
			&key.LastUsedAt,
			&key.RevokedAt,
			&key.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api keys: %w", err)
	}
	return keys, nil
}

func (r *PostgresRepository) GetAPIKeyByID(ctx context.Context, keyID string) (APIKey, error) {
	q := `
SELECT id, merchant_id, secret_hash, mode, scope, status, allowed_ips, last_used_at, revoked_at, created_at
FROM paygate_merchants.api_keys
WHERE id = $1`

	var key APIKey
	if err := r.db.QueryRow(ctx, q, keyID).Scan(
		&key.ID,
		&key.MerchantID,
		&key.SecretHash,
		&key.Mode,
		&key.Scope,
		&key.Status,
		&key.AllowedIPs,
		&key.LastUsedAt,
		&key.RevokedAt,
		&key.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return APIKey{}, ErrAPIKeyNotFound
		}
		return APIKey{}, fmt.Errorf("get api key by id: %w", err)
	}

	return key, nil
}

func (r *PostgresRepository) CountActiveAPIKeysByMerchant(ctx context.Context, merchantID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
SELECT COUNT(*)
FROM paygate_merchants.api_keys
WHERE merchant_id = $1 AND status = 'active'
`, merchantID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count active api keys: %w", err)
	}
	return count, nil
}

func (r *PostgresRepository) UpdateAPIKeyLastUsed(ctx context.Context, keyID string) error {
	_, err := r.db.Exec(ctx, `UPDATE paygate_merchants.api_keys SET last_used_at = NOW() WHERE id = $1`, keyID)
	if err != nil {
		return fmt.Errorf("update api key last_used_at: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RevokeAPIKey(ctx context.Context, merchantID, keyID string) error {
	q := `
UPDATE paygate_merchants.api_keys
SET status = 'revoked', revoked_at = NOW()
WHERE merchant_id = $1 AND id = $2 AND status = 'active'`
	cmd, err := r.db.Exec(ctx, q, merchantID, keyID)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

func (r *PostgresRepository) CreateMerchantUser(ctx context.Context, user MerchantUser) (MerchantUser, error) {
	if err := r.db.QueryRow(ctx, `
INSERT INTO paygate_merchants.merchant_users (id, merchant_id, email, password_hash, role, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING created_at, updated_at
`, user.ID, user.MerchantID, user.Email, user.PasswordHash, user.Role, user.Status).Scan(&user.CreatedAt, &user.UpdatedAt); err != nil {
		return MerchantUser{}, fmt.Errorf("insert merchant user: %w", err)
	}
	return user, nil
}

func (r *PostgresRepository) GetMerchantUserByID(ctx context.Context, userID string) (MerchantUser, error) {
	var user MerchantUser
	err := r.db.QueryRow(ctx, `
SELECT id, merchant_id, email, password_hash, role, status, last_login_at, created_at, updated_at
FROM paygate_merchants.merchant_users
WHERE id = $1
`, userID).Scan(
		&user.ID,
		&user.MerchantID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.LastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MerchantUser{}, ErrMerchantUserNotFound
		}
		return MerchantUser{}, fmt.Errorf("get merchant user by id: %w", err)
	}
	return user, nil
}

func (r *PostgresRepository) GetMerchantUserByMerchantAndEmail(ctx context.Context, merchantID, email string) (MerchantUser, error) {
	var user MerchantUser
	err := r.db.QueryRow(ctx, `
SELECT id, merchant_id, email, password_hash, role, status, last_login_at, created_at, updated_at
FROM paygate_merchants.merchant_users
WHERE merchant_id = $1 AND email = $2
`, merchantID, email).Scan(
		&user.ID,
		&user.MerchantID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.LastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MerchantUser{}, ErrMerchantUserNotFound
		}
		return MerchantUser{}, fmt.Errorf("get merchant user by email: %w", err)
	}
	return user, nil
}

func (r *PostgresRepository) CountMerchantUsersByMerchant(ctx context.Context, merchantID string) (int, error) {
	var count int
	if err := r.db.QueryRow(ctx, `
SELECT COUNT(*)
FROM paygate_merchants.merchant_users
WHERE merchant_id = $1
`, merchantID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count merchant users: %w", err)
	}
	return count, nil
}

func (r *PostgresRepository) UpdateMerchantUserLastLogin(ctx context.Context, userID string) error {
	if _, err := r.db.Exec(ctx, `
UPDATE paygate_merchants.merchant_users
SET last_login_at = NOW(), updated_at = NOW()
WHERE id = $1
`, userID); err != nil {
		return fmt.Errorf("update merchant user last login: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CreateInvitation(ctx context.Context, inv Invitation) (Invitation, error) {
	q := `
INSERT INTO paygate_merchants.merchant_invitations
    (id, merchant_id, email, role, token_hash, status, invited_by, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING created_at, updated_at`

	if err := r.db.QueryRow(ctx, q,
		inv.ID, inv.MerchantID, inv.Email, inv.Role,
		inv.TokenHash, inv.Status, inv.InvitedBy, inv.ExpiresAt,
	).Scan(&inv.CreatedAt, &inv.UpdatedAt); err != nil {
		return Invitation{}, fmt.Errorf("insert invitation: %w", err)
	}
	return inv, nil
}

func (r *PostgresRepository) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (Invitation, error) {
	q := `
SELECT id, merchant_id, email, role, token_hash, status, invited_by,
       expires_at, accepted_at, created_at, updated_at
FROM paygate_merchants.merchant_invitations
WHERE token_hash = $1`

	var inv Invitation
	err := r.db.QueryRow(ctx, q, tokenHash).Scan(
		&inv.ID, &inv.MerchantID, &inv.Email, &inv.Role, &inv.TokenHash,
		&inv.Status, &inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt,
		&inv.CreatedAt, &inv.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invitation{}, ErrInvitationNotFound
		}
		return Invitation{}, fmt.Errorf("get invitation by token hash: %w", err)
	}
	return inv, nil
}

func (r *PostgresRepository) ListInvitationsByMerchant(ctx context.Context, merchantID string) ([]Invitation, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, email, role, token_hash, status, invited_by,
       expires_at, accepted_at, created_at, updated_at
FROM paygate_merchants.merchant_invitations
WHERE merchant_id = $1
ORDER BY created_at DESC
`, merchantID)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	defer rows.Close()

	var invs []Invitation
	for rows.Next() {
		var inv Invitation
		if err := rows.Scan(
			&inv.ID, &inv.MerchantID, &inv.Email, &inv.Role, &inv.TokenHash,
			&inv.Status, &inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt,
			&inv.CreatedAt, &inv.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan invitation: %w", err)
		}
		invs = append(invs, inv)
	}
	return invs, rows.Err()
}

func (r *PostgresRepository) MarkInvitationAccepted(ctx context.Context, invitationID string) error {
	_, err := r.db.Exec(ctx, `
UPDATE paygate_merchants.merchant_invitations
SET status = 'accepted', accepted_at = NOW(), updated_at = NOW()
WHERE id = $1`, invitationID)
	if err != nil {
		return fmt.Errorf("mark invitation accepted: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RevokeInvitation(ctx context.Context, merchantID, invitationID string) error {
	cmd, err := r.db.Exec(ctx, `
UPDATE paygate_merchants.merchant_invitations
SET status = 'revoked', updated_at = NOW()
WHERE merchant_id = $1 AND id = $2 AND status = 'pending'`, merchantID, invitationID)
	if err != nil {
		return fmt.Errorf("revoke invitation: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrInvitationNotFound
	}
	return nil
}

func (r *PostgresRepository) UpdateAPIKeyAllowedIPs(ctx context.Context, merchantID, keyID string, ips []string) error {
	cmd, err := r.db.Exec(ctx, `
UPDATE paygate_merchants.api_keys
SET allowed_ips = $3
WHERE merchant_id = $1 AND id = $2 AND status = 'active'`, merchantID, keyID, ips)
	if err != nil {
		return fmt.Errorf("update api key allowed ips: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}
