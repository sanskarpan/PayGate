package risk

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunAsyncDropsWhenQueueIsFull(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, WithAsyncConfig(1, time.Second))
	var calls atomic.Int32
	release := make(chan struct{})
	ev := RiskEvent{MerchantID: "merch_1", PaymentID: "pay_1"}

	svc.runAsync(context.Background(), "first", ev, func(ctx context.Context) error {
		calls.Add(1)
		<-release
		return nil
	})

	deadline := time.Now().Add(time.Second)
	for calls.Load() != 1 {
		if time.Now().After(deadline) {
			t.Fatal("expected first async callback to start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	svc.runAsync(context.Background(), "second", ev, func(ctx context.Context) error {
		calls.Add(1)
		return nil
	})
	time.Sleep(50 * time.Millisecond)

	if calls.Load() != 1 {
		t.Fatalf("expected second async callback to be dropped when queue is full, got %d calls", calls.Load())
	}
	close(release)
}
