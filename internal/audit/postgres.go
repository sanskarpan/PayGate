package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, log Log) (Log, error) {
	log.ID = idgen.New("alog")
	if log.Changes == nil {
		log.Changes = map[string]any{}
	}
	changesJSON, err := json.Marshal(log.Changes)
	if err != nil {
		return Log{}, fmt.Errorf("marshal audit changes: %w", err)
	}

	q := `
INSERT INTO paygate_audit.audit_logs
    (id, merchant_id, actor_id, actor_email, actor_type, action,
     resource_type, resource_id, changes, ip_address, correlation_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING created_at`

	if err := r.db.QueryRow(ctx, q,
		log.ID, log.MerchantID, log.ActorID, log.ActorEmail, log.ActorType,
		log.Action, log.ResourceType, log.ResourceID,
		changesJSON, log.IPAddress, log.CorrelationID,
	).Scan(&log.CreatedAt); err != nil {
		return Log{}, fmt.Errorf("insert audit log: %w", err)
	}
	return log, nil
}

func (r *PostgresRepository) List(ctx context.Context, in ListInput) ([]Log, error) {
	limit := in.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	q := `
SELECT id, merchant_id, actor_id, actor_email, actor_type, action,
       resource_type, resource_id, changes, ip_address, correlation_id, created_at
FROM paygate_audit.audit_logs
WHERE merchant_id = $1
  AND ($2 = '' OR actor_id = $2)
  AND ($3 = '' OR resource_type = $3)
  AND ($4 = '' OR resource_id = $4)
ORDER BY created_at DESC, id DESC
LIMIT $5`

	rows, err := r.db.Query(ctx, q, in.MerchantID, in.ActorID, in.ResourceType, in.ResourceID, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()

	var logs []Log
	for rows.Next() {
		var l Log
		var changesJSON []byte
		if err := rows.Scan(
			&l.ID, &l.MerchantID, &l.ActorID, &l.ActorEmail, &l.ActorType,
			&l.Action, &l.ResourceType, &l.ResourceID,
			&changesJSON, &l.IPAddress, &l.CorrelationID, &l.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		if len(changesJSON) > 0 {
			if err := json.Unmarshal(changesJSON, &l.Changes); err != nil {
				return nil, fmt.Errorf("unmarshal audit changes: %w", err)
			}
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}
