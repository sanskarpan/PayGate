package audit

import (
	"context"
	"sync"
	"testing"
	"time"
)

type blockingRepo struct {
	mu      sync.Mutex
	calls   int
	release chan struct{}
}

func (r *blockingRepo) Create(ctx context.Context, log Log) (Log, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	select {
	case <-r.release:
	case <-ctx.Done():
		return Log{}, ctx.Err()
	}
	return log, nil
}

func (r *blockingRepo) List(context.Context, ListInput) ([]Log, error) {
	return nil, nil
}

func TestRecordDropsWhenAsyncQueueIsFull(t *testing.T) {
	t.Parallel()

	repo := &blockingRepo{release: make(chan struct{})}
	svc := NewService(repo, nil, WithAsyncConfig(1, time.Second))

	svc.Record(context.Background(), RecordInput{MerchantID: "merch_1", Action: "first"})

	deadline := time.Now().Add(time.Second)
	for {
		repo.mu.Lock()
		calls := repo.calls
		repo.mu.Unlock()
		if calls == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected first async audit record to start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	svc.Record(context.Background(), RecordInput{MerchantID: "merch_1", Action: "second"})
	time.Sleep(50 * time.Millisecond)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.calls != 1 {
		t.Fatalf("expected second audit record to be dropped when queue is full, got %d calls", repo.calls)
	}
	close(repo.release)
}
