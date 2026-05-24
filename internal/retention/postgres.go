package retention

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) ListPolicies(ctx context.Context) ([]Policy, error) {
	rows, err := r.db.Query(ctx, `
SELECT artifact_class, action, retain_days, enabled, updated_by, created_at, updated_at
FROM paygate_ops.retention_policies
ORDER BY artifact_class ASC`)
	if err != nil {
		return nil, fmt.Errorf("list retention policies: %w", err)
	}
	defer rows.Close()
	var out []Policy
	for rows.Next() {
		var item Policy
		if err := rows.Scan(&item.ArtifactClass, &item.Action, &item.RetainDays, &item.Enabled, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan retention policy: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpsertPolicy(ctx context.Context, policy Policy) (Policy, error) {
	if policy.RetainDays < 0 {
		policy.RetainDays = 0
	}
	if strings.TrimSpace(policy.UpdatedBy) == "" {
		policy.UpdatedBy = "system"
	}
	_, err := r.db.Exec(ctx, `
INSERT INTO paygate_ops.retention_policies
    (artifact_class, action, retain_days, enabled, updated_by)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (artifact_class) DO UPDATE
SET action = EXCLUDED.action,
    retain_days = EXCLUDED.retain_days,
    enabled = EXCLUDED.enabled,
    updated_by = EXCLUDED.updated_by,
    updated_at = NOW()`,
		policy.ArtifactClass, policy.Action, policy.RetainDays, policy.Enabled, policy.UpdatedBy,
	)
	if err != nil {
		return Policy{}, fmt.Errorf("upsert retention policy: %w", err)
	}
	var out Policy
	err = r.db.QueryRow(ctx, `
SELECT artifact_class, action, retain_days, enabled, updated_by, created_at, updated_at
FROM paygate_ops.retention_policies
WHERE artifact_class = $1`, policy.ArtifactClass).
		Scan(&out.ArtifactClass, &out.Action, &out.RetainDays, &out.Enabled, &out.UpdatedBy, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return Policy{}, fmt.Errorf("get retention policy: %w", err)
	}
	return out, nil
}

func (r *PostgresRepository) ListLegalHolds(ctx context.Context, limit int) ([]LegalHold, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
SELECT id, artifact_class, COALESCE(merchant_id, ''), COALESCE(artifact_id, ''), reason, created_by, created_at, released_at
FROM paygate_ops.legal_holds
ORDER BY created_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list legal holds: %w", err)
	}
	defer rows.Close()
	var out []LegalHold
	for rows.Next() {
		var item LegalHold
		if err := rows.Scan(&item.ID, &item.ArtifactClass, &item.MerchantID, &item.ArtifactID, &item.Reason, &item.CreatedBy, &item.CreatedAt, &item.ReleasedAt); err != nil {
			return nil, fmt.Errorf("scan legal hold: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CreateLegalHold(ctx context.Context, in CreateLegalHoldInput) (LegalHold, error) {
	item := LegalHold{
		ID:            idgen.New("lhold"),
		ArtifactClass: in.ArtifactClass,
		MerchantID:    strings.TrimSpace(in.MerchantID),
		ArtifactID:    strings.TrimSpace(in.ArtifactID),
		Reason:        strings.TrimSpace(in.Reason),
		CreatedBy:     strings.TrimSpace(in.CreatedBy),
	}
	if item.Reason == "" {
		item.Reason = "manual hold"
	}
	_, err := r.db.Exec(ctx, `
INSERT INTO paygate_ops.legal_holds
    (id, artifact_class, merchant_id, artifact_id, reason, created_by)
VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6)`,
		item.ID, item.ArtifactClass, item.MerchantID, item.ArtifactID, item.Reason, item.CreatedBy,
	)
	if err != nil {
		return LegalHold{}, fmt.Errorf("create legal hold: %w", err)
	}
	err = r.db.QueryRow(ctx, `
SELECT id, artifact_class, COALESCE(merchant_id, ''), COALESCE(artifact_id, ''), reason, created_by, created_at, released_at
FROM paygate_ops.legal_holds
WHERE id = $1`, item.ID).
		Scan(&item.ID, &item.ArtifactClass, &item.MerchantID, &item.ArtifactID, &item.Reason, &item.CreatedBy, &item.CreatedAt, &item.ReleasedAt)
	if err != nil {
		return LegalHold{}, fmt.Errorf("load legal hold: %w", err)
	}
	return item, nil
}

func (r *PostgresRepository) ReleaseLegalHold(ctx context.Context, holdID string) (LegalHold, error) {
	var item LegalHold
	err := r.db.QueryRow(ctx, `
UPDATE paygate_ops.legal_holds
SET released_at = NOW()
WHERE id = $1
RETURNING id, artifact_class, COALESCE(merchant_id, ''), COALESCE(artifact_id, ''), reason, created_by, created_at, released_at`,
		holdID,
	).Scan(&item.ID, &item.ArtifactClass, &item.MerchantID, &item.ArtifactID, &item.Reason, &item.CreatedBy, &item.CreatedAt, &item.ReleasedAt)
	if err != nil {
		return LegalHold{}, fmt.Errorf("release legal hold: %w", err)
	}
	return item, nil
}

func (r *PostgresRepository) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
SELECT id, artifact_class, action, status, affected_count, error_message, actor_type, actor_id, started_at, completed_at
FROM paygate_ops.retention_runs
ORDER BY started_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list retention runs: %w", err)
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var item Run
		if err := rows.Scan(&item.ID, &item.ArtifactClass, &item.Action, &item.Status, &item.AffectedCount, &item.ErrorMessage, &item.ActorType, &item.ActorID, &item.StartedAt, &item.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan retention run: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) RunAll(ctx context.Context, actorType, actorID string) ([]Run, error) {
	policies, err := r.ListPolicies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Run, 0, len(policies))
	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}
		run, err := r.RunPolicy(ctx, RunInput{ArtifactClass: policy.ArtifactClass, ActorType: actorType, ActorID: actorID})
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, nil
}

func (r *PostgresRepository) RunPolicy(ctx context.Context, in RunInput) (Run, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Run{}, fmt.Errorf("begin retention run: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var policy Policy
	err = tx.QueryRow(ctx, `
SELECT artifact_class, action, retain_days, enabled, updated_by, created_at, updated_at
FROM paygate_ops.retention_policies
WHERE artifact_class = $1`, in.ArtifactClass).
		Scan(&policy.ArtifactClass, &policy.Action, &policy.RetainDays, &policy.Enabled, &policy.UpdatedBy, &policy.CreatedAt, &policy.UpdatedAt)
	if err != nil {
		return Run{}, fmt.Errorf("load retention policy: %w", err)
	}

	run := Run{
		ID:            idgen.New("rtrun"),
		ArtifactClass: policy.ArtifactClass,
		Action:        policy.Action,
		Status:        "started",
		ActorType:     strings.TrimSpace(in.ActorType),
		ActorID:       strings.TrimSpace(in.ActorID),
	}
	if run.ActorType == "" {
		run.ActorType = "system"
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_ops.retention_runs
    (id, artifact_class, action, status, actor_type, actor_id)
VALUES ($1, $2, $3, $4, $5, $6)`,
		run.ID, run.ArtifactClass, run.Action, run.Status, run.ActorType, run.ActorID,
	); err != nil {
		return Run{}, fmt.Errorf("insert retention run: %w", err)
	}

	var affected int64
	if policy.Enabled {
		affected, err = r.executePolicy(ctx, tx, policy)
		if err != nil {
			_, _ = tx.Exec(ctx, `
UPDATE paygate_ops.retention_runs
SET status = 'failed',
    error_message = $2,
    completed_at = NOW()
WHERE id = $1`, run.ID, err.Error())
			return Run{}, fmt.Errorf("execute retention policy: %w", err)
		}
	}

	err = tx.QueryRow(ctx, `
UPDATE paygate_ops.retention_runs
SET status = 'completed',
    affected_count = $2,
    completed_at = NOW()
WHERE id = $1
RETURNING id, artifact_class, action, status, affected_count, error_message, actor_type, actor_id, started_at, completed_at`,
		run.ID, affected,
	).Scan(&run.ID, &run.ArtifactClass, &run.Action, &run.Status, &run.AffectedCount, &run.ErrorMessage, &run.ActorType, &run.ActorID, &run.StartedAt, &run.CompletedAt)
	if err != nil {
		return Run{}, fmt.Errorf("complete retention run: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Run{}, fmt.Errorf("commit retention run: %w", err)
	}
	return run, nil
}

func (r *PostgresRepository) executePolicy(ctx context.Context, tx pgx.Tx, policy Policy) (int64, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(policy.RetainDays) * 24 * time.Hour)
	var tag pgconn.CommandTag
	var err error
	switch policy.ArtifactClass {
	case ArtifactClassReportExport:
		tag, err = tx.Exec(ctx, `
UPDATE paygate_reporting.export_jobs e
SET content_text = '',
    download_token = '',
    file_size_bytes = 0,
    retention_status = 'redacted',
    retention_updated_at = NOW()
WHERE e.created_at < $1
  AND e.retention_status = 'active'
  AND NOT EXISTS (
      SELECT 1
      FROM paygate_ops.legal_holds h
      WHERE h.artifact_class = $2
        AND h.released_at IS NULL
        AND (h.merchant_id IS NULL OR h.merchant_id = e.merchant_id)
        AND (h.artifact_id IS NULL OR h.artifact_id = e.id)
  )`, cutoff, policy.ArtifactClass)
	case ArtifactClassWebhookDeliveryAttempt:
		tag, err = tx.Exec(ctx, `
UPDATE paygate_webhooks.webhook_delivery_attempts w
SET request_body = NULL,
    response_body = NULL,
    error_message = CASE WHEN COALESCE(error_message, '') = '' THEN '' ELSE '[redacted]' END,
    retention_status = 'redacted',
    retention_updated_at = NOW()
WHERE w.created_at < $1
  AND w.retention_status = 'active'
  AND NOT EXISTS (
      SELECT 1
      FROM paygate_ops.legal_holds h
      WHERE h.artifact_class = $2
        AND h.released_at IS NULL
        AND (h.merchant_id IS NULL OR h.merchant_id = w.merchant_id)
        AND (h.artifact_id IS NULL OR h.artifact_id = w.id)
  )`, cutoff, policy.ArtifactClass)
	case ArtifactClassOnboardingDocument:
		tag, err = tx.Exec(ctx, `
UPDATE paygate_merchants.merchant_onboarding_documents d
SET storage_key = '',
    retention_status = 'redacted',
    retention_updated_at = NOW()
WHERE d.created_at < $1
  AND d.retention_status = 'active'
  AND NOT EXISTS (
      SELECT 1
      FROM paygate_ops.legal_holds h
      WHERE h.artifact_class = $2
        AND h.released_at IS NULL
        AND (h.merchant_id IS NULL OR h.merchant_id = d.merchant_id)
        AND (h.artifact_id IS NULL OR h.artifact_id = d.id)
  )`, cutoff, policy.ArtifactClass)
	default:
		return 0, fmt.Errorf("unsupported artifact class %q", policy.ArtifactClass)
	}
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
