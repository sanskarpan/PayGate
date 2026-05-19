package eventschema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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

func (r *PostgresRepository) CreateSchema(ctx context.Context, in CreateSchemaInput) (Schema, error) {
	id := idgen.New("esch")
	row := r.db.QueryRow(ctx, `
INSERT INTO paygate_schema.event_schemas (id, subject, event_type, topic_name, owner, review_link)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, subject, event_type, topic_name, owner, review_link, created_at, updated_at
`, id, in.Subject, in.EventType, in.TopicName, in.Owner, in.ReviewLink)
	return scanSchema(row)
}

func (r *PostgresRepository) ListSchemas(ctx context.Context) ([]Schema, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, subject, event_type, topic_name, owner, review_link, created_at, updated_at
FROM paygate_schema.event_schemas
ORDER BY subject ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Schema
	for rows.Next() {
		item, err := scanSchema(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetSchema(ctx context.Context, subject string) (Schema, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, subject, event_type, topic_name, owner, review_link, created_at, updated_at
FROM paygate_schema.event_schemas
WHERE subject = $1
`, subject)
	item, err := scanSchema(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Schema{}, ErrSchemaNotFound
	}
	return item, err
}

func (r *PostgresRepository) CreateVersion(ctx context.Context, in CreateVersionInput, checks []CompatibilityCheck) (Version, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Version{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	schemaJSON, err := json.Marshal(in.Schema)
	if err != nil {
		return Version{}, err
	}
	samplePayload, err := json.Marshal(in.SamplePayload)
	if err != nil {
		return Version{}, err
	}

	compatSummary := "initial version"
	compatDetails := map[string]any{}
	if len(checks) > 0 {
		compatSummary = checks[0].Summary
		compatDetails = map[string]any{"checks": checks}
	}
	compatJSON, err := json.Marshal(compatDetails)
	if err != nil {
		return Version{}, err
	}

	versionID := idgen.New("esv")
	row := tx.QueryRow(ctx, `
INSERT INTO paygate_schema.schema_versions
    (id, subject, version, status, schema_json, sample_payload, review_link, compatibility_summary, compatibility_details)
VALUES
    ($1, $2, $3, 'draft', $4, $5, $6, $7, $8)
RETURNING id, subject, version, status, schema_json, sample_payload, review_link, compatibility_summary,
          compatibility_details, activated_at, deprecated_at, created_at, updated_at
`, versionID, in.Subject, in.Version, schemaJSON, samplePayload, in.ReviewLink, compatSummary, compatJSON)

	version, err := scanVersion(row)
	if err != nil {
		return Version{}, err
	}

	for _, check := range checks {
		detailsJSON, err := json.Marshal(check.Details)
		if err != nil {
			return Version{}, err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO paygate_schema.schema_compatibility_checks
    (id, subject, candidate_version, baseline_version, check_type, compatible, summary, details_json)
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8)
`, idgen.New("eschk"), in.Subject, in.Version, check.BaselineVersion, check.CheckType, check.Compatible, check.Summary, detailsJSON); err != nil {
			return Version{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Version{}, err
	}
	return version, nil
}

func (r *PostgresRepository) ListVersions(ctx context.Context, subject string) ([]Version, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, subject, version, status, schema_json, sample_payload, review_link, compatibility_summary,
       compatibility_details, activated_at, deprecated_at, created_at, updated_at
FROM paygate_schema.schema_versions
WHERE subject = $1
ORDER BY created_at ASC
`, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Version
	for rows.Next() {
		item, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetVersion(ctx context.Context, subject, version string) (Version, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, subject, version, status, schema_json, sample_payload, review_link, compatibility_summary,
       compatibility_details, activated_at, deprecated_at, created_at, updated_at
FROM paygate_schema.schema_versions
WHERE subject = $1 AND version = $2
`, subject, version)
	item, err := scanVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrSchemaVersionNotFound
	}
	return item, err
}

func (r *PostgresRepository) GetActiveVersion(ctx context.Context, subject string) (Version, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, subject, version, status, schema_json, sample_payload, review_link, compatibility_summary,
       compatibility_details, activated_at, deprecated_at, created_at, updated_at
FROM paygate_schema.schema_versions
WHERE subject = $1 AND status = 'active'
ORDER BY activated_at DESC NULLS LAST, created_at DESC
LIMIT 1
`, subject)
	item, err := scanVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNoActiveSchemaVersion
	}
	return item, err
}

func (r *PostgresRepository) ActivateVersion(ctx context.Context, in ActivateVersionInput) (Version, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Version{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
UPDATE paygate_schema.schema_versions
SET status = 'deprecated', deprecated_at = NOW(), updated_at = NOW()
WHERE subject = $1 AND status = 'active'
`, in.Subject); err != nil {
		return Version{}, err
	}

	row := tx.QueryRow(ctx, `
UPDATE paygate_schema.schema_versions
SET status = 'active', activated_at = NOW(), deprecated_at = NULL, updated_at = NOW()
WHERE subject = $1 AND version = $2
RETURNING id, subject, version, status, schema_json, sample_payload, review_link, compatibility_summary,
          compatibility_details, activated_at, deprecated_at, created_at, updated_at
`, in.Subject, in.Version)
	version, err := scanVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrSchemaVersionNotFound
	}
	if err != nil {
		return Version{}, err
	}

	if _, err := tx.Exec(ctx, `
UPDATE paygate_schema.schema_rollouts
SET status = 'completed', updated_at = NOW()
WHERE subject = $1 AND status = 'dual_publish' AND to_version = $2
`, in.Subject, in.Version); err != nil {
		return Version{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Version{}, err
	}
	return version, nil
}

func (r *PostgresRepository) ListChecks(ctx context.Context, subject string, limit int) ([]CompatibilityCheck, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
SELECT id, subject, candidate_version, baseline_version, check_type, compatible, summary, details_json, created_at
FROM paygate_schema.schema_compatibility_checks
WHERE subject = $1
ORDER BY created_at DESC
LIMIT $2
`, subject, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CompatibilityCheck
	for rows.Next() {
		item, err := scanCheck(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CreateRollout(ctx context.Context, in CreateRolloutInput) (Rollout, error) {
	id := idgen.New("roll")
	row := r.db.QueryRow(ctx, `
INSERT INTO paygate_schema.schema_rollouts
    (id, subject, from_version, to_version, status, cutover_deadline, notes)
VALUES
    ($1, $2, $3, $4, 'dual_publish', $5, $6)
RETURNING id, subject, from_version, to_version, status, cutover_deadline, notes, created_at, updated_at
`, id, in.Subject, in.FromVersion, in.ToVersion, in.CutoverDeadline, in.Notes)
	item, err := scanRollout(row)
	if err != nil {
		return Rollout{}, err
	}
	return item, nil
}

func (r *PostgresRepository) GetRollout(ctx context.Context, rolloutID string) (Rollout, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, subject, from_version, to_version, status, cutover_deadline, notes, created_at, updated_at
FROM paygate_schema.schema_rollouts
WHERE id = $1
`, rolloutID)
	item, err := scanRollout(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Rollout{}, ErrSchemaRolloutNotFound
	}
	if err != nil {
		return Rollout{}, err
	}
	consumers, err := r.listRolloutConsumers(ctx, rolloutID)
	if err != nil {
		return Rollout{}, err
	}
	item.Consumers = consumers
	return item, nil
}

func (r *PostgresRepository) GetActiveRollout(ctx context.Context, subject string) (Rollout, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, subject, from_version, to_version, status, cutover_deadline, notes, created_at, updated_at
FROM paygate_schema.schema_rollouts
WHERE subject = $1 AND status = 'dual_publish'
ORDER BY created_at DESC
LIMIT 1
`, subject)
	item, err := scanRollout(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Rollout{}, ErrSchemaRolloutNotFound
	}
	if err != nil {
		return Rollout{}, err
	}
	consumers, err := r.listRolloutConsumers(ctx, item.ID)
	if err != nil {
		return Rollout{}, err
	}
	item.Consumers = consumers
	return item, nil
}

func (r *PostgresRepository) AckRollout(ctx context.Context, in AckRolloutInput) (RolloutConsumer, error) {
	id := idgen.New("rack")
	row := r.db.QueryRow(ctx, `
INSERT INTO paygate_schema.schema_rollout_consumers
    (id, rollout_id, consumer_name, acknowledged_version)
VALUES
    ($1, $2, $3, $4)
ON CONFLICT (rollout_id, consumer_name)
DO UPDATE SET acknowledged_version = EXCLUDED.acknowledged_version, acknowledged_at = NOW()
RETURNING id, rollout_id, consumer_name, acknowledged_version, acknowledged_at, created_at
`, id, in.RolloutID, in.ConsumerName, in.AcknowledgedVersion)
	item, err := scanRolloutConsumer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return RolloutConsumer{}, ErrSchemaRolloutNotFound
	}
	return item, err
}

func (r *PostgresRepository) listRolloutConsumers(ctx context.Context, rolloutID string) ([]RolloutConsumer, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, rollout_id, consumer_name, acknowledged_version, acknowledged_at, created_at
FROM paygate_schema.schema_rollout_consumers
WHERE rollout_id = $1
ORDER BY consumer_name ASC
`, rolloutID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RolloutConsumer
	for rows.Next() {
		item, err := scanRolloutConsumer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListDeprecatedVersionAlerts(ctx context.Context) ([]DeprecatedVersionAlert, error) {
	rows, err := r.db.Query(ctx, `
SELECT r.id, r.subject, r.from_version, r.to_version, c.consumer_name, c.acknowledged_version, r.cutover_deadline
FROM paygate_schema.schema_rollouts r
JOIN paygate_schema.schema_rollout_consumers c ON c.rollout_id = r.id
WHERE r.status = 'dual_publish'
  AND r.cutover_deadline IS NOT NULL
  AND r.cutover_deadline < NOW()
  AND c.acknowledged_version = r.from_version
ORDER BY r.cutover_deadline ASC, r.subject ASC, c.consumer_name ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DeprecatedVersionAlert
	for rows.Next() {
		var item DeprecatedVersionAlert
		if err := rows.Scan(&item.RolloutID, &item.Subject, &item.FromVersion, &item.ToVersion, &item.ConsumerName, &item.AcknowledgedVersion, &item.CutoverDeadline); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

type schemaScannable interface {
	Scan(dest ...any) error
}

func scanSchema(row schemaScannable) (Schema, error) {
	var item Schema
	err := row.Scan(&item.ID, &item.Subject, &item.EventType, &item.TopicName, &item.Owner, &item.ReviewLink, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func scanVersion(row schemaScannable) (Version, error) {
	var item Version
	var schemaJSON []byte
	var sampleJSON []byte
	var detailsJSON []byte
	err := row.Scan(
		&item.ID, &item.Subject, &item.Version, &item.Status, &schemaJSON, &sampleJSON, &item.ReviewLink,
		&item.CompatibilitySummary, &detailsJSON, &item.ActivatedAt, &item.DeprecatedAt, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return Version{}, err
	}
	if err := json.Unmarshal(schemaJSON, &item.Schema); err != nil {
		return Version{}, fmt.Errorf("unmarshal schema document: %w", err)
	}
	if err := json.Unmarshal(sampleJSON, &item.SamplePayload); err != nil {
		return Version{}, fmt.Errorf("unmarshal schema sample payload: %w", err)
	}
	if err := json.Unmarshal(detailsJSON, &item.CompatibilityDetails); err != nil {
		return Version{}, fmt.Errorf("unmarshal compatibility details: %w", err)
	}
	return item, nil
}

func scanCheck(row schemaScannable) (CompatibilityCheck, error) {
	var item CompatibilityCheck
	var detailsJSON []byte
	err := row.Scan(&item.ID, &item.Subject, &item.CandidateVersion, &item.BaselineVersion, &item.CheckType, &item.Compatible, &item.Summary, &detailsJSON, &item.CreatedAt)
	if err != nil {
		return CompatibilityCheck{}, err
	}
	if err := json.Unmarshal(detailsJSON, &item.Details); err != nil {
		return CompatibilityCheck{}, err
	}
	return item, nil
}

func scanRollout(row schemaScannable) (Rollout, error) {
	var item Rollout
	err := row.Scan(&item.ID, &item.Subject, &item.FromVersion, &item.ToVersion, &item.Status, &item.CutoverDeadline, &item.Notes, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func scanRolloutConsumer(row schemaScannable) (RolloutConsumer, error) {
	var item RolloutConsumer
	err := row.Scan(&item.ID, &item.RolloutID, &item.ConsumerName, &item.AcknowledgedVersion, &item.AcknowledgedAt, &item.CreatedAt)
	return item, err
}
