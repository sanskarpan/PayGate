package webhook

import "context"

// Repository defines the storage operations for the webhook service.
type Repository interface {
	// Subscription CRUD
	CreateSubscription(ctx context.Context, in CreateInput) (WebhookSubscription, error)
	GetSubscription(ctx context.Context, merchantID, id string) (WebhookSubscription, error)
	ListSubscriptions(ctx context.Context, merchantID string) ([]WebhookSubscription, error)
	UpdateSubscription(ctx context.Context, merchantID, id string, in UpdateInput) (WebhookSubscription, error)
	TransitionSubscription(ctx context.Context, merchantID, id string, ev SubscriptionEvent) (WebhookSubscription, error)
	DeleteSubscription(ctx context.Context, merchantID, id string) error

	// Active subscriptions matching a given event type for a given merchant
	FindActiveSubscriptions(ctx context.Context, merchantID, eventType string) ([]WebhookSubscription, error)

	// Delivery attempt recording
	CreateDeliveryAttempt(ctx context.Context, attempt WebhookDeliveryAttempt) (WebhookDeliveryAttempt, error)
	UpdateDeliveryAttempt(ctx context.Context, id string, status DeliveryStatus, responseCode int, responseBody, errMsg string, nextRetryAt *string) (WebhookDeliveryAttempt, error)
	ListDeliveryAttempts(ctx context.Context, merchantID, subscriptionID string) ([]WebhookDeliveryAttempt, error)
	GetDeliveryAttempt(ctx context.Context, id string) (WebhookDeliveryAttempt, error)

	// Pending retries: delivery attempts that have failed and have a next_retry_at in the past
	PendingRetries(ctx context.Context, limit int) ([]WebhookDeliveryAttempt, error)

	// Event deduplication: returns true if the (event_id, subscription_id) pair was already delivered
	IsDelivered(ctx context.Context, eventID, subscriptionID string) (bool, error)
}
