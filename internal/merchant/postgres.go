package merchant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/common/protect"
)

type PostgresRepository struct {
	db        *pgxpool.Pool
	protector *protect.Protector
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db, protector: protect.Default()}
}

func (r *PostgresRepository) CreateMerchant(ctx context.Context, merchant Merchant) (Merchant, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Merchant{}, fmt.Errorf("begin create merchant: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

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

	row := tx.QueryRow(ctx, q, merchant.ID, merchant.Name, merchant.Email, merchant.BusinessType, merchant.Status, settingsJSON)
	if err := row.Scan(&merchant.CreatedAt, &merchant.UpdatedAt); err != nil {
		return Merchant{}, fmt.Errorf("insert merchant: %w", err)
	}

	app := OnboardingApplication{
		ID:          fmt.Sprintf("kyb_%s", merchant.ID),
		MerchantID:  merchant.ID,
		CountryCode: "IN",
		State:       OnboardingStateDraft,
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_merchants.merchant_onboarding_applications
(id, merchant_id, country_code, state)
VALUES ($1, $2, $3, $4)
`, app.ID, app.MerchantID, app.CountryCode, app.State); err != nil {
		return Merchant{}, fmt.Errorf("insert onboarding application: %w", err)
	}
	if err := r.insertOnboardingEventTx(ctx, tx, app.ID, merchant.ID, "merchant.created", "system", "system", "", string(OnboardingStateDraft), map[string]any{
		"merchant_id": merchant.ID,
	}); err != nil {
		return Merchant{}, err
	}
	for _, capability := range DefaultCapabilities(merchant.ID) {
		if _, err := tx.Exec(ctx, `
INSERT INTO paygate_merchants.merchant_capabilities
    (id, merchant_id, capability_code, status, reason, updated_by)
VALUES ($1, $2, $3, $4, $5, $6)
`, idgen.New("mcap"), merchant.ID, capability.CapabilityCode, capability.Status, capability.Reason, "system"); err != nil {
			return Merchant{}, fmt.Errorf("insert merchant capability: %w", err)
		}
	}
	policy := DefaultReservePolicy(merchant.ID)
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_merchants.merchant_reserve_policies
    (id, merchant_id, policy_type, percentage_bps, hold_days, threshold_amount, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`, idgen.New("mrsv"), merchant.ID, policy.PolicyType, policy.PercentageBPS, policy.HoldDays, policy.ThresholdAmount, policy.Notes); err != nil {
		return Merchant{}, fmt.Errorf("insert reserve policy: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Merchant{}, fmt.Errorf("commit create merchant: %w", err)
	}

	merchant.Settings = settings
	merchant.OnboardingStatus = OnboardingStateDraft
	return merchant, nil
}

func (r *PostgresRepository) GetMerchantByID(ctx context.Context, merchantID string) (Merchant, error) {
	q := `
SELECT m.id, m.name, m.email, m.business_type, m.status, m.settings, COALESCE(oa.state, 'draft'), m.created_at, m.updated_at
FROM paygate_merchants.merchants m
LEFT JOIN paygate_merchants.merchant_onboarding_applications oa ON oa.merchant_id = m.id
WHERE m.id = $1`

	var m Merchant
	var rawSettings []byte
	if err := r.db.QueryRow(ctx, q, merchantID).Scan(
		&m.ID,
		&m.Name,
		&m.Email,
		&m.BusinessType,
		&m.Status,
		&rawSettings,
		&m.OnboardingStatus,
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

func (r *PostgresRepository) GetOnboardingApplicationByMerchant(ctx context.Context, merchantID string) (OnboardingApplication, error) {
	var app OnboardingApplication
	err := r.db.QueryRow(ctx, `
SELECT id, merchant_id, legal_name, business_classification, registration_number, tax_identifier, country_code, state, reviewer_notes,
       submitted_at, reviewed_at, approved_at, rejected_at, created_at, updated_at
FROM paygate_merchants.merchant_onboarding_applications
WHERE merchant_id = $1
`, merchantID).Scan(
		&app.ID,
		&app.MerchantID,
		&app.LegalName,
		&app.BusinessClassification,
		&app.RegistrationNumber,
		&app.TaxIdentifier,
		&app.CountryCode,
		&app.State,
		&app.ReviewerNotes,
		&app.SubmittedAt,
		&app.ReviewedAt,
		&app.ApprovedAt,
		&app.RejectedAt,
		&app.CreatedAt,
		&app.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OnboardingApplication{}, ErrOnboardingApplicationNotFound
		}
		return OnboardingApplication{}, fmt.Errorf("get onboarding application: %w", err)
	}
	app.RegistrationNumber, err = r.protector.OpenString(app.RegistrationNumber)
	if err != nil {
		return OnboardingApplication{}, fmt.Errorf("decrypt registration number: %w", err)
	}
	app.TaxIdentifier, err = r.protector.OpenString(app.TaxIdentifier)
	if err != nil {
		return OnboardingApplication{}, fmt.Errorf("decrypt tax identifier: %w", err)
	}
	return app, nil
}

func (r *PostgresRepository) UpsertOnboardingApplication(ctx context.Context, app OnboardingApplication, actor, actorScope string) (OnboardingApplication, error) {
	current, err := r.GetOnboardingApplicationByMerchant(ctx, app.MerchantID)
	if err != nil {
		return OnboardingApplication{}, err
	}
	registrationNumber, err := r.protector.SealString(app.RegistrationNumber)
	if err != nil {
		return OnboardingApplication{}, fmt.Errorf("encrypt registration number: %w", err)
	}
	taxIdentifier, err := r.protector.SealString(app.TaxIdentifier)
	if err != nil {
		return OnboardingApplication{}, fmt.Errorf("encrypt tax identifier: %w", err)
	}
	if _, err := r.db.Exec(ctx, `
UPDATE paygate_merchants.merchant_onboarding_applications
SET legal_name = $2,
    business_classification = $3,
    registration_number = $4,
    tax_identifier = $5,
    country_code = $6,
    state = $7,
    reviewer_notes = $8,
    updated_at = NOW()
WHERE merchant_id = $1
`, app.MerchantID, app.LegalName, app.BusinessClassification, registrationNumber, taxIdentifier, app.CountryCode, app.State, app.ReviewerNotes); err != nil {
		return OnboardingApplication{}, fmt.Errorf("update onboarding application: %w", err)
	}
	if err := r.insertOnboardingEvent(ctx, current.ID, current.MerchantID, "application.updated", actor, actorScope, string(current.State), string(app.State), map[string]any{
		"legal_name":              app.LegalName,
		"business_classification": app.BusinessClassification,
		"registration_number":     "[protected]",
		"tax_identifier":          "[protected]",
		"country_code":            app.CountryCode,
	}); err != nil {
		return OnboardingApplication{}, err
	}
	return r.GetOnboardingApplicationByMerchant(ctx, app.MerchantID)
}

func (r *PostgresRepository) TransitionOnboardingApplication(ctx context.Context, merchantID string, nextState OnboardingState, reviewerNotes, actor, actorScope string) (OnboardingApplication, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return OnboardingApplication{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current OnboardingApplication
	if err := tx.QueryRow(ctx, `
SELECT id, merchant_id, legal_name, business_classification, registration_number, tax_identifier, country_code, state, reviewer_notes,
       submitted_at, reviewed_at, approved_at, rejected_at, created_at, updated_at
FROM paygate_merchants.merchant_onboarding_applications
WHERE merchant_id = $1
FOR UPDATE
`, merchantID).Scan(
		&current.ID,
		&current.MerchantID,
		&current.LegalName,
		&current.BusinessClassification,
		&current.RegistrationNumber,
		&current.TaxIdentifier,
		&current.CountryCode,
		&current.State,
		&current.ReviewerNotes,
		&current.SubmittedAt,
		&current.ReviewedAt,
		&current.ApprovedAt,
		&current.RejectedAt,
		&current.CreatedAt,
		&current.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OnboardingApplication{}, ErrOnboardingApplicationNotFound
		}
		return OnboardingApplication{}, fmt.Errorf("lock onboarding application: %w", err)
	}

	now := time.Now().UTC()
	submittedAt := current.SubmittedAt
	reviewedAt := current.ReviewedAt
	approvedAt := current.ApprovedAt
	rejectedAt := current.RejectedAt
	switch nextState {
	case OnboardingStateSubmitted:
		submittedAt = &now
		reviewedAt, approvedAt, rejectedAt = nil, nil, nil
	case OnboardingStateInReview, OnboardingStateNeedsInformation:
		reviewedAt = &now
		approvedAt, rejectedAt = nil, nil
	case OnboardingStateApproved:
		reviewedAt = &now
		approvedAt = &now
		rejectedAt = nil
	case OnboardingStateRejected:
		reviewedAt = &now
		rejectedAt = &now
		approvedAt = nil
	default:
		return OnboardingApplication{}, ErrInvalidOnboardingState
	}

	if _, err := tx.Exec(ctx, `
UPDATE paygate_merchants.merchant_onboarding_applications
SET state = $2,
    reviewer_notes = $3,
    submitted_at = $4,
    reviewed_at = $5,
    approved_at = $6,
    rejected_at = $7,
    updated_at = NOW()
WHERE merchant_id = $1
`, merchantID, nextState, reviewerNotes, submittedAt, reviewedAt, approvedAt, rejectedAt); err != nil {
		return OnboardingApplication{}, fmt.Errorf("transition onboarding application: %w", err)
	}
	if err := r.insertOnboardingEventTx(ctx, tx, current.ID, merchantID, "application.transition", actor, actorScope, string(current.State), string(nextState), map[string]any{
		"reviewer_notes": reviewerNotes,
	}); err != nil {
		return OnboardingApplication{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return OnboardingApplication{}, err
	}
	return r.GetOnboardingApplicationByMerchant(ctx, merchantID)
}

func (r *PostgresRepository) insertOnboardingEvent(ctx context.Context, applicationID, merchantID, eventType, actor, actorScope, before, after string, payload map[string]any) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := r.insertOnboardingEventTx(ctx, tx, applicationID, merchantID, eventType, actor, actorScope, before, after, payload); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) insertOnboardingEventTx(ctx context.Context, tx pgx.Tx, applicationID, merchantID, eventType, actor, actorScope, before, after string, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal onboarding event payload: %w", err)
	}
	_, err = tx.Exec(ctx, `
INSERT INTO paygate_merchants.merchant_onboarding_events
(id, application_id, merchant_id, event_type, actor, actor_scope, state_before, state_after, payload)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
`, idgen.New("kybev"), applicationID, merchantID, eventType, actor, actorScope, before, after, raw)
	if err != nil {
		return fmt.Errorf("insert onboarding event: %w", err)
	}
	return nil
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
