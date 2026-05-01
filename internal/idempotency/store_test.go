//go:build integration

package idempotency_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/idempotency"
)

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PAYGATE_TEST_DB_URL")
	if dsn == "" {
		dsn = "postgres://paygate:paygate@localhost:5432/paygate?sslmode=disable"
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skip integration: db unavailable: %v", err)
	}
	if err := db.Ping(ctx); err != nil {
		t.Skipf("skip integration: db ping failed: %v", err)
	}
	return db
}

func TestStoreNewRequest(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	store := idempotency.NewStore(db, nil)
	merchantID := "merch_idem_new"
	endpointHash, requestHash := idempotency.HashRequest("POST", "/v1/orders", []byte(`{"amount":1000}`))

	decision, err := store.Start(ctx, merchantID, endpointHash, "key-new-1", requestHash)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if decision.Bypass {
		t.Fatal("expected no Bypass for valid merchant and key")
	}
	if decision.Replay {
		t.Fatal("expected no Replay for new request")
	}
	if decision.InProgress {
		t.Fatal("expected no InProgress for new request")
	}
	if decision.Conflict {
		t.Fatal("expected no Conflict for new request")
	}
}

func TestStoreReplayCompletedRequest(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	store := idempotency.NewStore(db, nil)
	merchantID := "merch_idem_replay"
	endpointHash, requestHash := idempotency.HashRequest("POST", "/v1/orders", []byte(`{"amount":2000}`))
	clientKey := "key-replay-1"

	// First call: start the request
	decision, err := store.Start(ctx, merchantID, endpointHash, clientKey, requestHash)
	if err != nil {
		t.Fatalf("Start (first): %v", err)
	}
	if decision.Replay || decision.Conflict || decision.InProgress {
		t.Fatalf("unexpected decision on first Start: %+v", decision)
	}

	// Complete the request
	if err := store.Complete(ctx, merchantID, endpointHash, clientKey, requestHash, "order", "ord_replay_1", 201, []byte(`{"id":"ord_replay_1"}`)); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// Second call with same key + same hash → should replay
	decision2, err := store.Start(ctx, merchantID, endpointHash, clientKey, requestHash)
	if err != nil {
		t.Fatalf("Start (second): %v", err)
	}
	if !decision2.Replay {
		t.Fatalf("expected Replay=true on second Start, got %+v", decision2)
	}
	if decision2.ResponseCode != 201 {
		t.Fatalf("expected ResponseCode=201, got %d", decision2.ResponseCode)
	}
	if len(decision2.ResponseBody) == 0 {
		t.Fatal("expected ResponseBody to be non-empty on replay")
	}
}

func TestStoreInProgressRequest(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	store := idempotency.NewStore(db, nil)
	merchantID := "merch_idem_inprog"
	endpointHash, requestHash := idempotency.HashRequest("POST", "/v1/orders", []byte(`{"amount":3000}`))
	clientKey := "key-inprog-1"

	// First call locks the record as in_progress
	_, err := store.Start(ctx, merchantID, endpointHash, clientKey, requestHash)
	if err != nil {
		t.Fatalf("Start (first): %v", err)
	}

	// Second call with same key + same hash while still in_progress → InProgress=true
	decision2, err := store.Start(ctx, merchantID, endpointHash, clientKey, requestHash)
	if err != nil {
		t.Fatalf("Start (second): %v", err)
	}
	if !decision2.InProgress {
		t.Fatalf("expected InProgress=true, got %+v", decision2)
	}
	if decision2.RetryAfter <= 0 {
		t.Fatalf("expected RetryAfter > 0, got %d", decision2.RetryAfter)
	}
}

func TestStoreConflictDifferentRequestHash(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	store := idempotency.NewStore(db, nil)
	merchantID := "merch_idem_conflict"
	endpointHash1, requestHash1 := idempotency.HashRequest("POST", "/v1/orders", []byte(`{"amount":4000}`))
	_, requestHash2 := idempotency.HashRequest("POST", "/v1/orders", []byte(`{"amount":9999}`))
	clientKey := "key-conflict-1"

	// First call with original request body
	_, err := store.Start(ctx, merchantID, endpointHash1, clientKey, requestHash1)
	if err != nil {
		t.Fatalf("Start (first): %v", err)
	}

	// Second call with same endpoint + same client key but different request body
	decision2, err := store.Start(ctx, merchantID, endpointHash1, clientKey, requestHash2)
	if err != nil {
		t.Fatalf("Start (second): %v", err)
	}
	if !decision2.Conflict {
		t.Fatalf("expected Conflict=true for different request hash, got %+v", decision2)
	}
}
