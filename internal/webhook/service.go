package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Service orchestrates webhook subscription management and event delivery.
type Service struct {
	repo      Repository
	deliverer *Deliverer
}

// NewService creates a new Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, deliverer: NewDeliverer()}
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
		// Skip if already delivered to this subscription (idempotency).
		delivered, err := s.repo.IsDelivered(ctx, eventID, sub.ID)
		if err != nil {
			return err
		}
		if delivered {
			continue
		}

		result := s.deliverer.Deliver(ctx, sub.URL, sub.Secret, eventType, body)

		status := DeliveryFailed
		if result.Succeeded {
			status = DeliverySucceeded
		}

		attempt := WebhookDeliveryAttempt{
			EventID:        eventID,
			SubscriptionID: sub.ID,
			MerchantID:     merchantID,
			Status:         status,
			RequestURL:     sub.URL,
			RequestBody:    body,
			ResponseCode:   result.StatusCode,
			ResponseBody:   result.ResponseBody,
			ErrorMessage:   result.Error,
			AttemptNumber:  1,
		}

		if status == DeliveryFailed {
			delay := RetryDelay(1)
			nextRetry := time.Now().Add(delay)
			attempt.NextRetryAt = &nextRetry
		}

		if _, err := s.repo.CreateDeliveryAttempt(ctx, attempt); err != nil {
			return fmt.Errorf("record delivery attempt: %w", err)
		}
	}
	return nil
}

// RetryPendingDeliveries polls for failed delivery attempts with due next_retry_at
// and re-delivers them. Returns the number of attempts processed.
func (s *Service) RetryPendingDeliveries(ctx context.Context, limit int) (int, error) {
	attempts, err := s.repo.PendingRetries(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("poll pending retries: %w", err)
	}

	for _, attempt := range attempts {
		sub, err := s.repo.GetSubscription(ctx, attempt.MerchantID, attempt.SubscriptionID)
		if err != nil {
			// Subscription may have been deleted; skip.
			continue
		}

		result := s.deliverer.Deliver(ctx, sub.URL, sub.Secret, "", attempt.RequestBody)

		nextAttempt := attempt.AttemptNumber + 1
		var (
			status      DeliveryStatus
			nextRetryAt *string
		)

		if result.Succeeded {
			status = DeliverySucceeded
		} else if nextAttempt > MaxDeliveryAttempts {
			status = DeliveryDeadLettered
		} else {
			status = DeliveryFailed
			delay := RetryDelay(nextAttempt)
			t := time.Now().Add(delay).Format(time.RFC3339)
			nextRetryAt = &t
		}

		if _, err := s.repo.UpdateDeliveryAttempt(
			ctx, attempt.ID, status,
			result.StatusCode, result.ResponseBody, result.Error, nextRetryAt,
		); err != nil {
			return 0, err
		}
	}
	return len(attempts), nil
}
