package billing

import (
	"errors"
	"testing"
)

// The endpoint reports which subscriptions failed, so a partial run can never
// be read as a clean one.
func TestFailedSubscriptionIDs(t *testing.T) {
	joined := errors.Join(
		errors.New("run subscription sub_one: subscription is not active"),
		errors.New("run subscription sub_two: subscription is not active"),
	)

	got := failedSubscriptionIDs(joined)
	if len(got) != 2 || got[0] != "sub_one" || got[1] != "sub_two" {
		t.Fatalf("expected [sub_one sub_two], got %v", got)
	}
}

func TestFailedSubscriptionIDsIsEmptyForACleanRun(t *testing.T) {
	if ids := failedSubscriptionIDs(nil); len(ids) != 0 {
		t.Fatalf("expected no ids for a clean run, got %v", ids)
	}
}

// A single unwrapped error still has to be reported.
func TestFailedSubscriptionIDsHandlesSingleError(t *testing.T) {
	got := failedSubscriptionIDs(errors.New("run subscription sub_solo: boom"))
	if len(got) != 1 || got[0] != "sub_solo" {
		t.Fatalf("expected [sub_solo], got %v", got)
	}
}
