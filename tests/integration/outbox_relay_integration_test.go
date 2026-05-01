//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/sanskarpan/PayGate/internal/outbox"
)

func TestOutboxRelayPublishBatchAndCleanup(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	// Clear unpublished rows so the count assertions below are deterministic.
	if _, err := db.Exec(ctx, `DELETE FROM public.outbox WHERE aggregate_id = 'pay_test_relay'`); err != nil {
		t.Fatalf("cleanup prior test rows: %v", err)
	}

	publisher := &fakePublisher{}
	relay := outbox.NewRelay(db, publisher, 0, nil)

	// Insert a row directly so we control exactly what is in the table.
	if _, err := db.Exec(ctx, `
INSERT INTO public.outbox (id, aggregate_type, aggregate_id, event_type, merchant_id, payload)
VALUES (gen_random_uuid()::text, 'payment', 'pay_test_relay', 'payment.captured', 'test_relay_merch', '{"test":true}')
`); err != nil {
		t.Fatalf("insert outbox row: %v", err)
	}

	// PublishBatch should process exactly the one row we inserted.
	count, err := relay.PublishBatch(ctx, 10)
	if err != nil {
		t.Fatalf("PublishBatch: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count=1, got %d", count)
	}

	// fakePublisher should have received the event on the payments topic.
	if len(publisher.topics) != 1 || publisher.topics[0] != "paygate.payments" {
		t.Fatalf("expected topic paygate.payments, got %#v", publisher.topics)
	}

	// The outbox row should now have published_at set.
	var publishedAt *time.Time
	if err := db.QueryRow(ctx, `SELECT published_at FROM public.outbox WHERE aggregate_id = 'pay_test_relay'`).Scan(&publishedAt); err != nil {
		t.Fatalf("query published_at: %v", err)
	}
	if publishedAt == nil {
		t.Fatal("expected published_at to be set after PublishBatch")
	}

	// CountUnpublished should return 0 (our row is now published).
	unpublished, err := relay.CountUnpublished(ctx)
	if err != nil {
		t.Fatalf("CountUnpublished: %v", err)
	}
	if unpublished != 0 {
		t.Fatalf("expected 0 unpublished rows, got %d", unpublished)
	}

	// CleanupPublished with olderThan=0 deletes all published rows including ours.
	deleted, err := relay.CleanupPublished(ctx, 0)
	if err != nil {
		t.Fatalf("CleanupPublished: %v", err)
	}
	if deleted < 1 {
		t.Fatalf("expected CleanupPublished to delete at least 1 row, got %d", deleted)
	}

	// Verify the row is gone.
	var remaining int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM public.outbox WHERE aggregate_id = 'pay_test_relay'`).Scan(&remaining); err != nil {
		t.Fatalf("count remaining rows: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected row to be deleted after CleanupPublished, got %d remaining", remaining)
	}
}
