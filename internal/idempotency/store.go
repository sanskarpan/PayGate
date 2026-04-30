package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

const (
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

type cachedResponse struct {
	RequestHash string          `json:"request_hash"`
	StatusCode  int             `json:"status_code"`
	Body        json.RawMessage `json:"body"`
}

type Decision struct {
	Bypass       bool
	Replay       bool
	InProgress   bool
	Conflict     bool
	RetryAfter   int
	ResponseCode int
	ResponseBody []byte
	EndpointHash string
}

type Store struct {
	db      *pgxpool.Pool
	redis   *redis.Client
	ttl     time.Duration
	lockTTL time.Duration
}

func NewStore(db *pgxpool.Pool, redisClient *redis.Client) *Store {
	return &Store{
		db:      db,
		redis:   redisClient,
		ttl:     24 * time.Hour,
		lockTTL: 30 * time.Second,
	}
}

func HashRequest(method, endpoint string, body []byte) (string, string) {
	endpointHash := sha256.Sum256([]byte(method + ":" + endpoint))
	requestHash := sha256.Sum256(append([]byte(method+":"+endpoint+":"), body...))
	return hex.EncodeToString(endpointHash[:]), hex.EncodeToString(requestHash[:])
}

func (s *Store) Start(ctx context.Context, merchantID, endpointHash, clientKey, requestHash string) (Decision, error) {
	if merchantID == "" || clientKey == "" {
		return Decision{Bypass: true}, nil
	}

	if cached, ok, err := s.getCached(ctx, merchantID, endpointHash, clientKey); err == nil && ok {
		if cached.RequestHash != requestHash {
			return Decision{Conflict: true}, nil
		}
		return Decision{
			Replay:       true,
			ResponseCode: cached.StatusCode,
			ResponseBody: append([]byte(nil), cached.Body...),
			EndpointHash: endpointHash,
		}, nil
	}

	now := time.Now().UTC()
	recordID := idgen.New("idem")
	cmd, err := s.db.Exec(ctx, `
INSERT INTO paygate_idempotency.idempotency_records
(id, merchant_id, endpoint_hash, client_key, request_hash, status, locked_until, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT DO NOTHING
`, recordID, merchantID, endpointHash, clientKey, requestHash, StatusInProgress, now.Add(s.lockTTL), now.Add(s.ttl))
	if err != nil {
		return Decision{}, fmt.Errorf("insert idempotency record: %w", err)
	}
	if cmd.RowsAffected() == 1 {
		return Decision{EndpointHash: endpointHash}, nil
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Decision{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	var storedHash string
	var lockedUntil *time.Time
	var responseCode *int
	var responseBody []byte
	err = tx.QueryRow(ctx, `
SELECT status, request_hash, locked_until, response_code, response_body
FROM paygate_idempotency.idempotency_records
WHERE merchant_id = $1 AND endpoint_hash = $2 AND client_key = $3
FOR UPDATE
`, merchantID, endpointHash, clientKey).Scan(&status, &storedHash, &lockedUntil, &responseCode, &responseBody)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Decision{EndpointHash: endpointHash}, nil
		}
		return Decision{}, fmt.Errorf("select idempotency record: %w", err)
	}
	if storedHash != requestHash {
		return Decision{Conflict: true}, nil
	}
	if status == StatusCompleted && responseCode != nil && len(responseBody) > 0 {
		if err := tx.Commit(ctx); err != nil {
			return Decision{}, err
		}
		return Decision{
			Replay:       true,
			ResponseCode: *responseCode,
			ResponseBody: responseBody,
			EndpointHash: endpointHash,
		}, nil
	}
	if status == StatusInProgress && lockedUntil != nil && lockedUntil.After(now) {
		retryAfter := int(time.Until(*lockedUntil).Seconds())
		if retryAfter < 1 {
			retryAfter = 1
		}
		return Decision{InProgress: true, RetryAfter: retryAfter, EndpointHash: endpointHash}, nil
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_idempotency.idempotency_records
SET status = $4, locked_until = $5, updated_at = NOW(), expires_at = $6
WHERE merchant_id = $1 AND endpoint_hash = $2 AND client_key = $3
`, merchantID, endpointHash, clientKey, StatusInProgress, now.Add(s.lockTTL), now.Add(s.ttl)); err != nil {
		return Decision{}, fmt.Errorf("reclaim idempotency record: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Decision{}, err
	}
	return Decision{EndpointHash: endpointHash}, nil
}

func (s *Store) Complete(ctx context.Context, merchantID, endpointHash, clientKey, requestHash, resourceType, resourceID string, statusCode int, body []byte) error {
	if merchantID == "" || clientKey == "" {
		return nil
	}
	bodyJSON, err := normalizeJSON(body)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `
UPDATE paygate_idempotency.idempotency_records
SET request_hash = $4,
    status = $5,
    resource_type = $6,
    resource_id = $7,
    response_code = $8,
    response_body = $9,
    locked_until = NULL,
    updated_at = NOW()
WHERE merchant_id = $1 AND endpoint_hash = $2 AND client_key = $3
`, merchantID, endpointHash, clientKey, requestHash, StatusCompleted, resourceType, resourceID, statusCode, bodyJSON); err != nil {
		return fmt.Errorf("complete idempotency record: %w", err)
	}
	return s.cache(ctx, merchantID, endpointHash, clientKey, cachedResponse{
		RequestHash: requestHash,
		StatusCode:  statusCode,
		Body:        bodyJSON,
	})
}

func (s *Store) Fail(ctx context.Context, merchantID, endpointHash, clientKey, requestHash string, statusCode int, body []byte) error {
	if merchantID == "" || clientKey == "" {
		return nil
	}
	bodyJSON, err := normalizeJSON(body)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
UPDATE paygate_idempotency.idempotency_records
SET request_hash = $4,
    status = $5,
    response_code = $6,
    response_body = $7,
    locked_until = NULL,
    updated_at = NOW()
WHERE merchant_id = $1 AND endpoint_hash = $2 AND client_key = $3
`, merchantID, endpointHash, clientKey, requestHash, StatusFailed, statusCode, bodyJSON)
	if err != nil {
		return fmt.Errorf("fail idempotency record: %w", err)
	}
	return nil
}

func (s *Store) cacheKey(merchantID, endpointHash, clientKey string) string {
	return fmt.Sprintf("idempotency:%s:%s:%s", merchantID, endpointHash, clientKey)
}

func (s *Store) getCached(ctx context.Context, merchantID, endpointHash, clientKey string) (cachedResponse, bool, error) {
	if s.redis == nil {
		return cachedResponse{}, false, nil
	}
	raw, err := s.redis.Get(ctx, s.cacheKey(merchantID, endpointHash, clientKey)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return cachedResponse{}, false, nil
		}
		return cachedResponse{}, false, err
	}
	var cached cachedResponse
	if err := json.Unmarshal(raw, &cached); err != nil {
		return cachedResponse{}, false, err
	}
	return cached, true, nil
}

func (s *Store) cache(ctx context.Context, merchantID, endpointHash, clientKey string, cached cachedResponse) error {
	if s.redis == nil {
		return nil
	}
	raw, err := json.Marshal(cached)
	if err != nil {
		return err
	}
	return s.redis.Set(ctx, s.cacheKey(merchantID, endpointHash, clientKey), raw, s.ttl).Err()
}

func normalizeJSON(body []byte) (json.RawMessage, error) {
	if len(body) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, fmt.Errorf("normalize idempotency response body: %w", err)
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(normalized), nil
}
