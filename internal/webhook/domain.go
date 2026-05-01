package webhook

import (
	"errors"
	"time"
)

// SubscriptionStatus is the explicit state machine type for webhook subscriptions.
type SubscriptionStatus string

// SubscriptionEvent drives transitions in the subscription state machine.
type SubscriptionEvent string

const (
	StatusActive   SubscriptionStatus = "active"
	StatusDisabled SubscriptionStatus = "disabled"
	StatusDeleted  SubscriptionStatus = "deleted"
)

const (
	EventEnable  SubscriptionEvent = "enable"
	EventDisable SubscriptionEvent = "disable"
	EventDelete  SubscriptionEvent = "delete"
)

// DeliveryStatus is the state of a single delivery attempt.
type DeliveryStatus string

const (
	DeliveryPending      DeliveryStatus = "pending"
	DeliverySucceeded    DeliveryStatus = "succeeded"
	DeliveryFailed       DeliveryStatus = "failed"
	DeliveryDeadLettered DeliveryStatus = "dead_lettered"
)

// MaxDeliveryAttempts is the number of attempts before an event is dead-lettered.
const MaxDeliveryAttempts = 18

// RotateSecretGracePeriod is the duration the previous secret remains valid
// after a rotation so consumers can update their verification code without
// downtime.
const RotateSecretGracePeriod = 24 * time.Hour

var (
	ErrSubscriptionNotFound    = errors.New("webhook subscription not found")
	ErrDeliveryAttemptNotFound = errors.New("webhook delivery attempt not found")
	ErrInvalidTransition       = errors.New("invalid subscription state transition")
	ErrInvalidURL              = errors.New("webhook URL must use https scheme")
	ErrNoEvents                = errors.New("at least one event type must be specified")
)

// Transition returns the next SubscriptionStatus for the given event,
// or ErrInvalidTransition if the event is not valid from the current state.
func Transition(from SubscriptionStatus, ev SubscriptionEvent) (SubscriptionStatus, error) {
	table := map[SubscriptionStatus]map[SubscriptionEvent]SubscriptionStatus{
		StatusActive: {
			EventDisable: StatusDisabled,
			EventDelete:  StatusDeleted,
		},
		StatusDisabled: {
			EventEnable: StatusActive,
			EventDelete: StatusDeleted,
		},
	}
	m, ok := table[from]
	if !ok {
		return "", ErrInvalidTransition
	}
	next, ok := m[ev]
	if !ok {
		return "", ErrInvalidTransition
	}
	return next, nil
}

// WebhookSubscription is a merchant's registered endpoint for receiving events.
type WebhookSubscription struct {
	ID         string
	MerchantID string
	URL        string
	Events     []string // event type patterns subscribed to, e.g. ["payment.captured"]
	Secret     string   // HMAC-SHA256 signing secret (never returned to client after creation)
	// PreviousSecret is the signing secret that was replaced by the most recent
	// rotation. It remains valid until PreviousSecretExpiresAt to allow a
	// grace-period overlap when consumers update their verification logic.
	// Nil when no rotation has occurred yet.
	PreviousSecret          *string
	PreviousSecretExpiresAt *time.Time
	Status                   SubscriptionStatus
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// WebhookDeliveryAttempt records one HTTP delivery attempt to a subscription.
type WebhookDeliveryAttempt struct {
	ID             string
	EventID        string
	SubscriptionID string
	MerchantID     string
	Status         DeliveryStatus
	RequestURL     string
	RequestBody    []byte
	ResponseCode   int
	ResponseBody   string
	ErrorMessage   string
	AttemptNumber  int
	NextRetryAt    *time.Time
	CreatedAt      time.Time
}

// RetryDelay returns the delay before the next attempt for a given attempt number.
// Attempt 1 is immediate (0 delay). This function is called for attempt N to get
// the delay before attempt N+1.
func RetryDelay(attemptNumber int) time.Duration {
	delays := []time.Duration{
		0,             // attempt 1: immediate (no delay before first try)
		5 * time.Second,
		10 * time.Second,
		30 * time.Second,
		1 * time.Minute,
		5 * time.Minute,
		10 * time.Minute,
		30 * time.Minute,
		1 * time.Hour,
		1 * time.Hour,
		1 * time.Hour,
		1 * time.Hour,
		1 * time.Hour,
		1 * time.Hour,
		1 * time.Hour,
		1 * time.Hour,
		1 * time.Hour,
		1 * time.Hour,
	}
	if attemptNumber <= 0 || attemptNumber > len(delays) {
		return 1 * time.Hour
	}
	return delays[attemptNumber-1] //nolint:gosec // bounds are checked on the line above
}

// MatchesEvent reports whether the subscription is subscribed to the given event type.
// An exact match or a wildcard prefix (e.g. "payment.*") are both valid.
func (s *WebhookSubscription) MatchesEvent(eventType string) bool {
	for _, pattern := range s.Events {
		if pattern == eventType {
			return true
		}
		// Wildcard: "payment.*" matches "payment.captured"
		if len(pattern) > 1 && pattern[len(pattern)-1] == '*' {
			prefix := pattern[:len(pattern)-1]
			if len(eventType) >= len(prefix) && eventType[:len(prefix)] == prefix {
				return true
			}
		}
	}
	return false
}

// CreateInput carries validated fields for creating a new subscription.
type CreateInput struct {
	MerchantID string
	URL        string
	Events     []string
}

// UpdateInput carries mutable fields for updating a subscription.
type UpdateInput struct {
	URL    string
	Events []string
}
