package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Publisher interface {
	Publish(ctx context.Context, topic string, key string, payload []byte) error
	Close() error
}

type Relay struct {
	db        *pgxpool.Pool
	publisher Publisher
	logger    *slog.Logger
	interval  time.Duration
	resolver  SchemaVersionResolver
	tableName string
}

type PublishSchemaVersion struct {
	Subject string
	Version string
}

type SchemaVersionResolver interface {
	ResolvePublishVersions(ctx context.Context, subject string) ([]PublishSchemaVersion, error)
}

type Record struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	MerchantID    string
	Payload       json.RawMessage
	CreatedAt     time.Time
}

func NewRelay(db *pgxpool.Pool, publisher Publisher, interval time.Duration, logger *slog.Logger) *Relay {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = time.Second
	}
	return &Relay{db: db, publisher: publisher, logger: logger, interval: interval, tableName: "public.outbox"}
}

func (r *Relay) WithSchemaVersionResolver(resolver SchemaVersionResolver) *Relay {
	r.resolver = resolver
	return r
}

func (r *Relay) WithTableName(tableName string) *Relay {
	if tableName != "" {
		r.tableName = tableName
	}
	return r
}

func (r *Relay) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	defer func() { _ = r.publisher.Close() }()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := r.PublishBatch(ctx, 100)
			if err != nil {
				r.logger.Error("outbox publish batch failed", "error", err)
				continue
			}
			if count > 0 {
				r.logger.Info("published outbox events", "count", count)
			}
			if _, err := r.CleanupPublished(ctx, 7*24*time.Hour); err != nil {
				r.logger.Error("outbox cleanup failed", "error", err)
			}
		}
	}
}

func (r *Relay) PublishBatch(ctx context.Context, limit int) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, fmt.Sprintf(`
SELECT id, aggregate_type, aggregate_id, event_type, merchant_id, payload, created_at
FROM %s
WHERE published_at IS NULL
ORDER BY created_at
LIMIT $1
FOR UPDATE SKIP LOCKED
`, r.tableName), limit)
	if err != nil {
		return 0, fmt.Errorf("query outbox batch: %w", err)
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ID, &rec.AggregateType, &rec.AggregateID, &rec.EventType, &rec.MerchantID, &rec.Payload, &rec.CreatedAt); err != nil {
			return 0, fmt.Errorf("scan outbox record: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, rec := range records {
		schemas, err := r.resolvePublishSchemas(ctx, rec.EventType)
		if err != nil {
			return 0, err
		}
		topic := TopicForEvent(rec.EventType)
		for _, schema := range schemas {
			payload, err := json.Marshal(map[string]any{
				"id":             rec.ID,
				"event_id":       rec.ID,
				"aggregate_type": rec.AggregateType,
				"aggregate_id":   rec.AggregateID,
				"event_type":     rec.EventType,
				"merchant_id":    rec.MerchantID,
				"payload":        json.RawMessage(rec.Payload),
				"created_at":     rec.CreatedAt.Unix(),
				"occurred_at":    rec.CreatedAt.UTC().Format(time.RFC3339),
				"correlation_id": rec.AggregateID,
				"causation_id":   rec.ID,
				"schema_subject": schema.Subject,
				"schema_version": schema.Version,
			})
			if err != nil {
				return 0, fmt.Errorf("marshal outbox envelope: %w", err)
			}
			if err := publishWithRetry(ctx, r.publisher, topic, rec.MerchantID, payload); err != nil {
				return 0, err
			}
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET published_at = NOW() WHERE id = $1`, r.tableName), rec.ID); err != nil {
			return 0, fmt.Errorf("mark outbox published: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(records), nil
}

func (r *Relay) resolvePublishSchemas(ctx context.Context, eventType string) ([]PublishSchemaVersion, error) {
	if r.resolver == nil {
		return []PublishSchemaVersion{{Subject: eventType, Version: "1.0.0"}}, nil
	}
	versions, err := r.resolver.ResolvePublishVersions(ctx, eventType)
	if err != nil {
		return nil, fmt.Errorf("resolve schema publish versions for %s: %w", eventType, err)
	}
	if len(versions) == 0 {
		return []PublishSchemaVersion{{Subject: eventType, Version: "1.0.0"}}, nil
	}
	return versions, nil
}

func (r *Relay) CleanupPublished(ctx context.Context, olderThan time.Duration) (int64, error) {
	cmd, err := r.db.Exec(ctx, fmt.Sprintf(`
DELETE FROM %s
WHERE published_at IS NOT NULL AND published_at < NOW() - ($1::interval)
`, r.tableName), fmt.Sprintf("%f seconds", olderThan.Seconds()))
	if err != nil {
		return 0, fmt.Errorf("cleanup outbox rows: %w", err)
	}
	return cmd.RowsAffected(), nil
}

func (r *Relay) CountUnpublished(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM %s
WHERE published_at IS NULL
`, r.tableName)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count unpublished outbox rows: %w", err)
	}
	return count, nil
}

func TopicForEvent(eventType string) string {
	switch {
	case strings.HasPrefix(eventType, "order."):
		return "paygate.orders"
	case strings.HasPrefix(eventType, "payment."):
		return "paygate.payments"
	case strings.HasPrefix(eventType, "refund."):
		return "paygate.refunds"
	case strings.HasPrefix(eventType, "settlement."):
		return "paygate.settlements"
	default:
		return "paygate.internal"
	}
}

func publishWithRetry(ctx context.Context, publisher Publisher, topic, key string, payload []byte) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := publisher.Publish(ctx, topic, key, payload); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("publish outbox event: %w", lastErr)
}
