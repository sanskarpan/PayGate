package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sanskarpan/PayGate/internal/common/metrics"
)

// Option is a functional option for configuring a Service.
type Option func(*Service)

// WithRedis attaches a Redis client to the Service for fast dedup fingerprint
// checks. Pass nil to disable Redis-backed dedup (the repo-level IsDelivered
// check still applies).
func WithRedis(client *redis.Client) Option {
	return func(s *Service) {
		s.redis = client
	}
}

// Service orchestrates webhook subscription management and event delivery.
type Service struct {
	repo      Repository
	deliverer *Deliverer
	redis     *redis.Client // nullable; nil means Redis dedup is disabled
}

// NewService creates a new Service, applying any functional options.
func NewService(repo Repository, opts ...Option) *Service {
	s := &Service{repo: repo, deliverer: NewDeliverer()}
	for _, o := range opts {
		o(s)
	}
	return s
}

// CreateSubscription creates a new webhook subscription.
func (s *Service) CreateSubscription(ctx context.Context, in CreateInput) (WebhookSubscription, error) {
	return s.repo.CreateSubscription(ctx, in)
}

// GetSubscription returns a subscription scoped to the merchant.
func (s *Service) GetSubscription(ctx context.Context, merchantID, id string) (WebhookSubscription, error) {
	return s.repo.GetSubscription(ctx, merchantID, id)
}

// ListSubscriptions returns all active subscriptions for a merchant.
func (s *Service) ListSubscriptions(ctx context.Context, merchantID string) ([]WebhookSubscription, error) {
	return s.repo.ListSubscriptions(ctx, merchantID)
}

// UpdateSubscription updates the URL and/or events of a subscription.
func (s *Service) UpdateSubscription(ctx context.Context, merchantID, id string, in UpdateInput) (WebhookSubscription, error) {
	return s.repo.UpdateSubscription(ctx, merchantID, id, in)
}

// DisableSubscription transitions a subscription from active → disabled.
func (s *Service) DisableSubscription(ctx context.Context, merchantID, id string) (WebhookSubscription, error) {
	return s.repo.TransitionSubscription(ctx, merchantID, id, EventDisable)
}

// EnableSubscription transitions a subscription from disabled → active.
func (s *Service) EnableSubscription(ctx context.Context, merchantID, id string) (WebhookSubscription, error) {
	return s.repo.TransitionSubscription(ctx, merchantID, id, EventEnable)
}

// DeleteSubscription soft-deletes a subscription.
func (s *Service) DeleteSubscription(ctx context.Context, merchantID, id string) error {
	return s.repo.DeleteSubscription(ctx, merchantID, id)
}

// ListDeliveryAttempts returns delivery attempts for a given subscription.
func (s *Service) ListDeliveryAttempts(ctx context.Context, merchantID, subscriptionID string) ([]WebhookDeliveryAttempt, error) {
	return s.repo.ListDeliveryAttempts(ctx, merchantID, subscriptionID)
}

// DeliverEvent finds matching active subscriptions for the event and delivers to each.
// Each delivery attempt is recorded regardless of outcome. This is the main delivery path
// called by the Kafka consumer after an outbox event is published.
func (s *Service) DeliverEvent(ctx context.Context, eventID, merchantID, eventType string, payload map[string]any) error {
	// Duplicate suppression at subscription level.
	subs, err := s.repo.FindActiveSubscriptions(ctx, merchantID, eventType)
	if err != nil {
		return fmt.Errorf("find subscriptions: %w", err)
	}
	if len(subs) == 0 {
		return nil
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	for _, sub := range subs {
		// Fast-path Redis dedup: check fingerprint key before hitting the DB.
		fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(eventID+":"+sub.ID)))
		redisKey := "webhook:dedup:" + fingerprint
		if s.redis != nil {
			exists, err := s.redis.Exists(ctx, redisKey).Result()
			if err == nil && exists > 0 {
				// Already marked as delivered in Redis — skip without a DB round-trip.
				continue
			}
		}

		attempt, err := s.repo.ReserveDeliveryAttempt(ctx, WebhookDeliveryAttempt{
			EventID:        eventID,
			EventType:      eventType,
			SubscriptionID: sub.ID,
			MerchantID:     merchantID,
			RequestURL:     sub.URL,
			RequestBody:    body,
		})
		if errors.Is(err, ErrDeliveryAlreadyReserved) {
			continue
		}
		if err != nil {
			return fmt.Errorf("reserve delivery attempt: %w", err)
		}

		result := s.deliverer.Deliver(ctx, sub.URL, sub.Secret, eventType, body, sub.SignatureMode)

		status := DeliveryFailed
		if result.Succeeded {
			status = DeliverySucceeded
		}
		metrics.WebhookSignatureDeliveriesTotal.WithLabelValues(string(sub.SignatureMode), string(status)).Inc()

		attempt.Status = status
		attempt.ResponseCode = result.StatusCode
		attempt.ResponseBody = result.ResponseBody
		attempt.ErrorMessage = result.Error
		if status == DeliveryFailed {
			delay := RetryDelay(2)
			nextRetry := time.Now().Add(delay)
			attempt.NextRetryAt = &nextRetry
		}

		var nextRetryAt *string
		if attempt.NextRetryAt != nil {
			value := attempt.NextRetryAt.Format(time.RFC3339)
			nextRetryAt = &value
		}
		if _, err := s.repo.UpdateDeliveryAttempt(
			ctx, attempt.ID, status, result.StatusCode, result.ResponseBody, result.Error, nextRetryAt, 1,
		); err != nil {
			return fmt.Errorf("finalize delivery attempt: %w", err)
		}

		// Persist the dedup fingerprint in Redis so subsequent consumers can
		// skip the DB check entirely for 48 hours.
		if s.redis != nil && result.Succeeded {
			// Best-effort: ignore errors so a Redis outage never blocks delivery.
			_ = s.redis.Set(ctx, redisKey, "1", 48*time.Hour).Err()
		}
	}
	return nil
}

// RotateSecret generates a new signing secret for the subscription.
// The new secret is returned in the response. The previous secret is immediately
// invalidated — callers must update their verification logic before rotating.
func (s *Service) RotateSecret(ctx context.Context, merchantID, id string) (WebhookSubscription, error) {
	return s.repo.RotateSecret(ctx, merchantID, id)
}

func (s *Service) CancelEventRetries(ctx context.Context, merchantID, eventID, reason string) (int64, error) {
	return s.repo.CancelDeliveryRetries(ctx, merchantID, eventID, reason)
}

// ReplayEvent re-delivers a previously recorded event to its matching subscriptions.
// It looks up all delivery attempts for the event, picks the request body from the
// most recent one, and re-calls DeliverEvent (which handles idempotency by checking
// for succeeded deliveries — so ReplayEvent explicitly clears that gate by using a
// new synthetic event ID derived from the original).
func (s *Service) ReplayEvent(ctx context.Context, merchantID, eventID string) (int, error) {
	attempts, err := s.repo.FindDeliveryByEvent(ctx, merchantID, eventID)
	if err != nil {
		return 0, fmt.Errorf("find event deliveries: %w", err)
	}
	if len(attempts) == 0 {
		return 0, ErrDeliveryAttemptNotFound
	}

	// Use the first attempt's request body (most recent by DESC ordering).
	body := attempts[0].RequestBody
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("unmarshal replay payload: %w", err)
	}

	// Use a replay-prefixed event ID so idempotency guard allows re-delivery.
	replayID := "replay_" + eventID
	eventType, _ := payload["event_type"].(string)

	if err := s.DeliverEvent(ctx, replayID, merchantID, eventType, payload); err != nil {
		return 0, err
	}
	return 1, nil
}

// RetryPendingDeliveries polls for failed delivery attempts with due next_retry_at
// and re-delivers them. Returns the number of attempts processed.
func (s *Service) RetryPendingDeliveries(ctx context.Context, limit int) (int, error) {
	attempts, err := s.repo.LeasePendingRetries(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("poll pending retries: %w", err)
	}

	for _, attempt := range attempts {
		sub, err := s.repo.GetSubscription(ctx, attempt.MerchantID, attempt.SubscriptionID)
		if err != nil {
			// Subscription may have been deleted; skip.
			continue
		}

		result := s.deliverer.Deliver(ctx, sub.URL, sub.Secret, attempt.EventType, attempt.RequestBody, sub.SignatureMode)

		nextAttempt := attempt.AttemptNumber + 1
		var (
			status      DeliveryStatus
			nextRetryAt *string
		)

		switch {
		case result.Succeeded:
			status = DeliverySucceeded
		case nextAttempt > MaxDeliveryAttempts:
			status = DeliveryDeadLettered
		default:
			status = DeliveryFailed
			delay := RetryDelay(nextAttempt + 1)
			t := time.Now().Add(delay).Format(time.RFC3339)
			nextRetryAt = &t
		}
		metrics.WebhookSignatureDeliveriesTotal.WithLabelValues(string(sub.SignatureMode), string(status)).Inc()

		if _, err := s.repo.UpdateDeliveryAttempt(
			ctx, attempt.ID, status,
			result.StatusCode, result.ResponseBody, result.Error, nextRetryAt, nextAttempt,
		); err != nil {
			return 0, err
		}
	}
	return len(attempts), nil
}
