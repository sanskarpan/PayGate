package billing

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// fakeDueRepo only implements the lease call. The embedded interface satisfies
// the remaining methods at compile time; anything else the code under test
// touches will panic loudly rather than silently returning a zero value.
type fakeDueRepo struct {
	Repository
	due      []Subscription
	leaseErr error
}

func (f *fakeDueRepo) LeaseDueSubscriptions(context.Context, time.Time, int) ([]Subscription, error) {
	return f.due, f.leaseErr
}

func newDueRunService(repo Repository) *Service {
	// Discard log output; these tests assert on returned errors, not logs.
	return NewService(repo, nil, nil, nil, WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
}

// A batch where every leased subscription fails must surface every failure, not
// report a clean run. Non-active subscriptions fail in runSubscription before it
// reaches the order or payment services.
func TestRunDueSubscriptionsJoinsPerSubscriptionFailures(t *testing.T) {
	repo := &fakeDueRepo{due: []Subscription{
		{ID: "sub_one", MerchantID: "merch_1", Status: SubscriptionPaused},
		{ID: "sub_two", MerchantID: "merch_1", Status: SubscriptionPaused},
	}}

	invoices, err := newDueRunService(repo).RunDueSubscriptions(context.Background(), 10)

	if err == nil {
		t.Fatal("expected the failed subscriptions to be reported, got a nil error")
	}
	if !errors.Is(err, ErrSubscriptionNotActive) {
		t.Fatalf("expected the underlying cause to be preserved, got %v", err)
	}
	for _, id := range []string{"sub_one", "sub_two"} {
		if !strings.Contains(err.Error(), id) {
			t.Fatalf("expected %s to be identified in %q", id, err.Error())
		}
	}
	if len(invoices) != 0 {
		t.Fatalf("expected no invoices when every subscription failed, got %d", len(invoices))
	}
}

// Nothing due is a clean run, not an error.
func TestRunDueSubscriptionsReturnsNilErrorWhenNothingDue(t *testing.T) {
	invoices, err := newDueRunService(&fakeDueRepo{}).RunDueSubscriptions(context.Background(), 10)
	if err != nil {
		t.Fatalf("expected no error when nothing is due, got %v", err)
	}
	if len(invoices) != 0 {
		t.Fatalf("expected no invoices, got %d", len(invoices))
	}
}

// A lease failure is a hard failure for the whole run.
func TestRunDueSubscriptionsPropagatesLeaseFailure(t *testing.T) {
	sentinel := errors.New("lease exploded")
	_, err := newDueRunService(&fakeDueRepo{leaseErr: sentinel}).RunDueSubscriptions(context.Background(), 10)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the lease error to propagate, got %v", err)
	}
}

// The handler reports which subscriptions failed; it must read them back out of
// the joined error and never report failures for a clean run.
func TestFailedSubscriptionIDs(t *testing.T) {
	joined := errors.Join(
		errors.New("subscription sub_one: not active"),
		errors.New("subscription sub_two: not active"),
	)
	got := failedSubscriptionIDs(joined)
	if len(got) != 2 || got[0] != "sub_one" || got[1] != "sub_two" {
		t.Fatalf("expected [sub_one sub_two], got %v", got)
	}
	if ids := failedSubscriptionIDs(nil); len(ids) != 0 {
		t.Fatalf("expected no ids for a clean run, got %v", ids)
	}
}
