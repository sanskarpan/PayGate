package webhook

import (
	"testing"
	"time"
)

func TestSubscriptionTransition(t *testing.T) {
	tests := []struct {
		from    SubscriptionStatus
		event   SubscriptionEvent
		want    SubscriptionStatus
		wantErr bool
	}{
		{StatusActive, EventDisable, StatusDisabled, false},
		{StatusActive, EventDelete, StatusDeleted, false},
		{StatusDisabled, EventEnable, StatusActive, false},
		{StatusDisabled, EventDelete, StatusDeleted, false},
		// Invalid transitions
		{StatusActive, EventEnable, "", true},
		{StatusDisabled, EventDisable, "", true},
		{StatusDeleted, EventEnable, "", true},
		{StatusDeleted, EventDisable, "", true},
		{StatusDeleted, EventDelete, "", true},
	}

	for _, tc := range tests {
		got, err := Transition(tc.from, tc.event)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Transition(%s, %s): expected error, got %s", tc.from, tc.event, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Transition(%s, %s): unexpected error: %v", tc.from, tc.event, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Transition(%s, %s) = %s, want %s", tc.from, tc.event, got, tc.want)
		}
	}
}

func TestMatchesEvent(t *testing.T) {
	sub := &WebhookSubscription{
		Events: []string{"payment.captured", "refund.*", "order.created"},
	}

	tests := []struct {
		eventType string
		want      bool
	}{
		{"payment.captured", true},
		{"payment.failed", false},
		{"refund.created", true},
		{"refund.processed", true},
		{"refund.failed", true},
		{"order.created", true},
		{"order.expired", false},
		{"settlement.processed", false},
		{"", false},
	}

	for _, tc := range tests {
		got := sub.MatchesEvent(tc.eventType)
		if got != tc.want {
			t.Errorf("MatchesEvent(%q) = %v, want %v", tc.eventType, got, tc.want)
		}
	}
}

func TestRetryDelay(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 0},
		{2, 5 * time.Second},
		{3, 10 * time.Second},
		{4, 30 * time.Second},
		{5, 1 * time.Minute},
		{6, 5 * time.Minute},
		{7, 10 * time.Minute},
		{8, 30 * time.Minute},
		{9, 1 * time.Hour},
		{18, 1 * time.Hour},
		{19, 1 * time.Hour}, // beyond table → clamp to 1h
		{0, 1 * time.Hour},  // invalid → clamp
	}

	for _, tc := range tests {
		got := RetryDelay(tc.attempt)
		if got != tc.want {
			t.Errorf("RetryDelay(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}
