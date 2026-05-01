package webhook

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

// PostgresRepository implements Repository using pgxpool.
type PostgresRepository struct {
	db *pgxpool.Pool
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateSubscription(ctx context.Context, in CreateInput) (WebhookSubscription, error) {
	if in.URL == "" {
		return WebhookSubscription{}, ErrInvalidURL
	}
	if len(in.Events) == 0 {
		return WebhookSubscription{}, ErrNoEvents
	}

	secret, err := generateSecret()
	if err != nil {
		return WebhookSubscription{}, err
	}

	sub := WebhookSubscription{
		ID:         idgen.New("whs"),
		MerchantID: in.MerchantID,
		URL:        in.URL,
		Events:     in.Events,
		Secret:     secret,
		Status:     StatusActive,
	}

	_, err = r.db.Exec(ctx, `
INSERT INTO paygate_webhooks.webhook_subscriptions
    (id, merchant_id, url, events, secret, status)
VALUES ($1, $2, $3, $4, $5, $6)
`, sub.ID, sub.MerchantID, sub.URL, sub.Events, sub.Secret, sub.Status)
	if err != nil {
		return WebhookSubscription{}, err
	}

	return r.GetSubscription(ctx, in.MerchantID, sub.ID)
}

func (r *PostgresRepository) GetSubscription(ctx context.Context, merchantID, id string) (WebhookSubscription, error) {
	var sub WebhookSubscription
	err := r.db.QueryRow(ctx, `
SELECT id, merchant_id, url, events, secret, status, created_at, updated_at
FROM paygate_webhooks.webhook_subscriptions
WHERE id = $1 AND merchant_id = $2 AND status != 'deleted'
`, id, merchantID).Scan(
		&sub.ID, &sub.MerchantID, &sub.URL, &sub.Events, &sub.Secret,
		&sub.Status, &sub.CreatedAt, &sub.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return WebhookSubscription{}, ErrSubscriptionNotFound
	}
	return sub, err
}

func (r *PostgresRepository) ListSubscriptions(ctx context.Context, merchantID string) ([]WebhookSubscription, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, url, events, secret, status, created_at, updated_at
FROM paygate_webhooks.webhook_subscriptions
WHERE merchant_id = $1 AND status != 'deleted'
ORDER BY created_at DESC
`, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []WebhookSubscription
	for rows.Next() {
		var sub WebhookSubscription
		if err := rows.Scan(&sub.ID, &sub.MerchantID, &sub.URL, &sub.Events, &sub.Secret,
			&sub.Status, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (r *PostgresRepository) UpdateSubscription(ctx context.Context, merchantID, id string, in UpdateInput) (WebhookSubscription, error) {
	// Verify it exists and is accessible.
	if _, err := r.GetSubscription(ctx, merchantID, id); err != nil {
		return WebhookSubscription{}, err
	}

	_, err := r.db.Exec(ctx, `
UPDATE paygate_webhooks.webhook_subscriptions
SET url = COALESCE(NULLIF($1, ''), url),
    events = CASE WHEN $2::text[] IS NOT NULL AND array_length($2::text[], 1) > 0 THEN $2 ELSE events END,
    updated_at = NOW()
WHERE id = $3 AND merchant_id = $4
`, in.URL, in.Events, id, merchantID)
	if err != nil {
		return WebhookSubscription{}, err
	}
	return r.GetSubscription(ctx, merchantID, id)
}

func (r *PostgresRepository) TransitionSubscription(ctx context.Context, merchantID, id string, ev SubscriptionEvent) (WebhookSubscription, error) {
	sub, err := r.GetSubscription(ctx, merchantID, id)
	if err != nil {
		return WebhookSubscription{}, err
	}
	next, err := Transition(sub.Status, ev)
	if err != nil {
		return WebhookSubscription{}, err
	}
	_, err = r.db.Exec(ctx, `
UPDATE paygate_webhooks.webhook_subscriptions
SET status = $1, updated_at = NOW()
WHERE id = $2 AND merchant_id = $3
`, next, id, merchantID)
	if err != nil {
		return WebhookSubscription{}, err
	}
	sub.Status = next
	return sub, nil
}

func (r *PostgresRepository) DeleteSubscription(ctx context.Context, merchantID, id string) error {
	_, err := r.TransitionSubscription(ctx, merchantID, id, EventDelete)
	return err
}

func (r *PostgresRepository) FindActiveSubscriptions(ctx context.Context, merchantID, eventType string) ([]WebhookSubscription, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, url, events, secret, status, created_at, updated_at
FROM paygate_webhooks.webhook_subscriptions
WHERE merchant_id = $1 AND status = 'active'
`, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []WebhookSubscription
	for rows.Next() {
		var sub WebhookSubscription
		if err := rows.Scan(&sub.ID, &sub.MerchantID, &sub.URL, &sub.Events, &sub.Secret,
			&sub.Status, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, err
		}
		if sub.MatchesEvent(eventType) {
			matches = append(matches, sub)
		}
	}
	return matches, rows.Err()
}

func (r *PostgresRepository) CreateDeliveryAttempt(ctx context.Context, attempt WebhookDeliveryAttempt) (WebhookDeliveryAttempt, error) {
	if attempt.ID == "" {
		attempt.ID = idgen.New("wdl")
	}
	_, err := r.db.Exec(ctx, `
INSERT INTO paygate_webhooks.webhook_delivery_attempts
    (id, event_id, subscription_id, merchant_id, status, request_url, request_body,
     response_code, response_body, error_message, attempt_number, next_retry_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
`,
		attempt.ID, attempt.EventID, attempt.SubscriptionID, attempt.MerchantID,
		attempt.Status, attempt.RequestURL, attempt.RequestBody,
		attempt.ResponseCode, attempt.ResponseBody, attempt.ErrorMessage,
		attempt.AttemptNumber, attempt.NextRetryAt,
	)
	if err != nil {
		return WebhookDeliveryAttempt{}, err
	}
	return attempt, nil
}

func (r *PostgresRepository) UpdateDeliveryAttempt(ctx context.Context, id string, status DeliveryStatus, responseCode int, responseBody, errMsg string, nextRetryAt *string) (WebhookDeliveryAttempt, error) {
	var nextRetry *time.Time
	if nextRetryAt != nil {
		t, err := time.Parse(time.RFC3339, *nextRetryAt)
		if err != nil {
			return WebhookDeliveryAttempt{}, err
		}
		nextRetry = &t
	}
	_, err := r.db.Exec(ctx, `
UPDATE paygate_webhooks.webhook_delivery_attempts
SET status = $1, response_code = $2, response_body = $3,
    error_message = $4, next_retry_at = $5
WHERE id = $6
`, status, responseCode, responseBody, errMsg, nextRetry, id)
	if err != nil {
		return WebhookDeliveryAttempt{}, err
	}
	return r.GetDeliveryAttempt(ctx, id)
}

func (r *PostgresRepository) GetDeliveryAttempt(ctx context.Context, id string) (WebhookDeliveryAttempt, error) {
	var a WebhookDeliveryAttempt
	err := r.db.QueryRow(ctx, `
SELECT id, event_id, subscription_id, merchant_id, status, request_url, request_body,
       response_code, response_body, error_message, attempt_number, next_retry_at, created_at
FROM paygate_webhooks.webhook_delivery_attempts
WHERE id = $1
`, id).Scan(
		&a.ID, &a.EventID, &a.SubscriptionID, &a.MerchantID, &a.Status,
		&a.RequestURL, &a.RequestBody, &a.ResponseCode, &a.ResponseBody, &a.ErrorMessage,
		&a.AttemptNumber, &a.NextRetryAt, &a.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return WebhookDeliveryAttempt{}, ErrDeliveryAttemptNotFound
	}
	return a, err
}

func (r *PostgresRepository) ListDeliveryAttempts(ctx context.Context, merchantID, subscriptionID string) ([]WebhookDeliveryAttempt, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, event_id, subscription_id, merchant_id, status, request_url, request_body,
       response_code, response_body, error_message, attempt_number, next_retry_at, created_at
FROM paygate_webhooks.webhook_delivery_attempts
WHERE merchant_id = $1 AND subscription_id = $2
ORDER BY created_at DESC
LIMIT 100
`, merchantID, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []WebhookDeliveryAttempt
	for rows.Next() {
		var a WebhookDeliveryAttempt
		if err := rows.Scan(&a.ID, &a.EventID, &a.SubscriptionID, &a.MerchantID, &a.Status,
			&a.RequestURL, &a.RequestBody, &a.ResponseCode, &a.ResponseBody, &a.ErrorMessage,
			&a.AttemptNumber, &a.NextRetryAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

func (r *PostgresRepository) PendingRetries(ctx context.Context, limit int) ([]WebhookDeliveryAttempt, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, event_id, subscription_id, merchant_id, status, request_url, request_body,
       response_code, response_body, error_message, attempt_number, next_retry_at, created_at
FROM paygate_webhooks.webhook_delivery_attempts
WHERE status = 'failed' AND next_retry_at IS NOT NULL AND next_retry_at <= NOW()
ORDER BY next_retry_at
LIMIT $1
FOR UPDATE SKIP LOCKED
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []WebhookDeliveryAttempt
	for rows.Next() {
		var a WebhookDeliveryAttempt
		if err := rows.Scan(&a.ID, &a.EventID, &a.SubscriptionID, &a.MerchantID, &a.Status,
			&a.RequestURL, &a.RequestBody, &a.ResponseCode, &a.ResponseBody, &a.ErrorMessage,
			&a.AttemptNumber, &a.NextRetryAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

func (r *PostgresRepository) IsDelivered(ctx context.Context, eventID, subscriptionID string) (bool, error) {
	var count int
	err := r.db.QueryRow(ctx, `
SELECT COUNT(*) FROM paygate_webhooks.webhook_delivery_attempts
WHERE event_id = $1 AND subscription_id = $2 AND status = 'succeeded'
`, eventID, subscriptionID).Scan(&count)
	return count > 0, err
}

// generateSecret returns a cryptographically random 32-byte hex string.
func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
