package outbox

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

type stubPublisher struct {
	err   error
	calls int
}

func (s *stubPublisher) Publish(_ context.Context, _ string, _ string, _ []byte) error {
	s.calls++
	return s.err
}

func (s *stubPublisher) Close() error { return nil }

func TestFallbackPublisherFallsBackOnPrimaryError(t *testing.T) {
	primary := &stubPublisher{err: errors.New("kafka unavailable")}
	fallbackCalls := 0
	fallback := NewLocalPublisher(func(topic, key string, payload []byte) error {
		fallbackCalls++
		if topic != "paygate.payments" || key != "merch_123" || string(payload) != `{"ok":true}` {
			t.Fatalf("unexpected fallback payload: topic=%s key=%s payload=%s", topic, key, string(payload))
		}
		return nil
	})

	publisher := NewFallbackPublisher(primary, fallback, slog.Default())
	if err := publisher.Publish(context.Background(), "paygate.payments", "merch_123", []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("publish returned error: %v", err)
	}

	if primary.calls != 1 {
		t.Fatalf("expected primary to be called once, got %d", primary.calls)
	}
	if fallbackCalls != 1 {
		t.Fatalf("expected fallback to be called once, got %d", fallbackCalls)
	}
}
