package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// S3Config holds the connection settings for the object storage backend.
// Compatible with both AWS S3 and MinIO (S3-compatible).
type S3Config struct {
	Endpoint  string // e.g. "http://localhost:9000" for MinIO, "" for AWS
	Bucket    string
	KeyPrefix string // e.g. "audit-logs/"
	AccessKey string
	SecretKey string
	Region    string
}

// Archiver moves audit log entries older than RetentionDays from Postgres
// into object storage (S3 / MinIO) in newline-delimited JSON format.
// It runs as a background goroutine, triggered daily.
type Archiver struct {
	db        *pgxpool.Pool
	s3        S3Config
	retention time.Duration // how old entries must be before archiving
	logger    *slog.Logger
}

// NewArchiver creates an Archiver. retention is typically 90 * 24 * time.Hour.
func NewArchiver(db *pgxpool.Pool, s3 S3Config, retention time.Duration, logger *slog.Logger) *Archiver {
	if logger == nil {
		logger = slog.Default()
	}
	if retention <= 0 {
		retention = 90 * 24 * time.Hour
	}
	return &Archiver{db: db, s3: s3, retention: retention, logger: logger}
}

// Start runs the archival loop daily until ctx is cancelled.
func (a *Archiver) Start(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	// Run immediately on start.
	a.runArchive(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.runArchive(ctx)
		}
	}
}

func (a *Archiver) runArchive(ctx context.Context) {
	n, err := a.Archive(ctx, time.Now().Add(-a.retention))
	if err != nil {
		a.logger.Error("audit log archival failed", "error", err)
		return
	}
	if n > 0 {
		a.logger.Info("audit logs archived", "count", n)
	}
}

// Archive fetches audit logs created before cutoff, uploads them to S3/MinIO
// as a NDJSON object keyed by date, and deletes the archived rows from Postgres.
// Returns the number of rows archived.
func (a *Archiver) Archive(ctx context.Context, cutoff time.Time) (int, error) {
	rows, err := a.db.Query(ctx, `
SELECT id, merchant_id, actor_id, actor_email, actor_type, action,
       resource_type, resource_id, changes, ip_address, correlation_id, created_at
FROM paygate_audit.audit_logs
WHERE created_at < $1
ORDER BY created_at
LIMIT 1000
`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("query audit logs for archival: %w", err)
	}
	defer rows.Close()

	var logs []Log
	var ids []string
	for rows.Next() {
		var l Log
		var changesRaw []byte
		if err := rows.Scan(
			&l.ID, &l.MerchantID, &l.ActorID, &l.ActorEmail, &l.ActorType,
			&l.Action, &l.ResourceType, &l.ResourceID, &changesRaw,
			&l.IPAddress, &l.CorrelationID, &l.CreatedAt,
		); err != nil {
			return 0, fmt.Errorf("scan audit log: %w", err)
		}
		if len(changesRaw) > 0 {
			_ = json.Unmarshal(changesRaw, &l.Changes)
		}
		logs = append(logs, l)
		ids = append(ids, l.ID)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(logs) == 0 {
		return 0, nil
	}

	// Encode as NDJSON.
	var buf bytes.Buffer
	for _, l := range logs {
		line, _ := json.Marshal(l)
		buf.Write(line)
		buf.WriteByte('\n')
	}

	// Upload to object storage.
	objectKey := fmt.Sprintf("%s%s.ndjson", a.s3.KeyPrefix, cutoff.Format("2006-01-02"))
	if err := a.upload(ctx, objectKey, buf.Bytes()); err != nil {
		return 0, fmt.Errorf("upload audit archive: %w", err)
	}

	// Delete archived rows.
	if _, err := a.db.Exec(ctx, `
DELETE FROM paygate_audit.audit_logs WHERE id = ANY($1)
`, ids); err != nil {
		return 0, fmt.Errorf("delete archived audit logs: %w", err)
	}
	return len(logs), nil
}

// upload performs a simple HTTP PUT to S3/MinIO. For production use, replace
// with the AWS SDK or minio-go. This implementation uses the S3 REST API directly.
func (a *Archiver) upload(ctx context.Context, objectKey string, data []byte) error {
	if a.s3.Endpoint == "" && a.s3.Bucket == "" {
		// No-op when storage is not configured (e.g. in tests).
		return nil
	}
	url := fmt.Sprintf("%s/%s/%s", a.s3.Endpoint, a.s3.Bucket, objectKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.SetBasicAuth(a.s3.AccessKey, a.s3.SecretKey)
	req.ContentLength = int64(len(data))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("s3 put request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("s3 put returned %d", resp.StatusCode)
	}
	return nil
}
