package webhook

import (
	"context"
	"errors"
	"testing"
)

type stubCatalog struct {
	known map[string]struct{}
	err   error
}

func (c stubCatalog) KnownEventTypes(context.Context) (map[string]struct{}, error) {
	return c.known, c.err
}

func catalogOf(events ...string) stubCatalog {
	known := make(map[string]struct{}, len(events))
	for _, e := range events {
		known[e] = struct{}{}
	}
	return stubCatalog{known: known}
}

// A subscription to an event the platform never publishes is accepted by the
// database but can never fire, so the merchant silently receives nothing.
func TestValidateEventTypesRejectsUnknownEvents(t *testing.T) {
	svc := NewService(nil, WithEventCatalog(catalogOf("payment.captured", "refund.processed")))

	err := svc.validateEventTypes(context.Background(), []string{"payment.captured", "not.a.real.event"})
	if !errors.Is(err, ErrUnknownEventType) {
		t.Fatalf("expected ErrUnknownEventType, got %v", err)
	}
	if err != nil && !containsString(err.Error(), "not.a.real.event") {
		t.Fatalf("the error should name the offending event, got %q", err)
	}
}

func TestValidateEventTypesAcceptsKnownEvents(t *testing.T) {
	svc := NewService(nil, WithEventCatalog(catalogOf("payment.captured", "refund.processed", "dispute.won")))
	if err := svc.validateEventTypes(context.Background(), []string{"payment.captured", "dispute.won"}); err != nil {
		t.Fatalf("known events must be accepted, got %v", err)
	}
}

// Without a catalog the behaviour is unchanged, so callers that have no
// registry are unaffected.
func TestValidateEventTypesSkippedWithoutCatalog(t *testing.T) {
	svc := NewService(nil)
	if err := svc.validateEventTypes(context.Background(), []string{"anything.at.all"}); err != nil {
		t.Fatalf("expected no validation without a catalog, got %v", err)
	}
}

// A registry outage must never block a merchant from configuring webhooks.
func TestValidateEventTypesFailsOpenWhenCatalogErrors(t *testing.T) {
	svc := NewService(nil, WithEventCatalog(stubCatalog{err: errors.New("registry down")}))
	if err := svc.validateEventTypes(context.Background(), []string{"whatever"}); err != nil {
		t.Fatalf("expected validation to be skipped on catalog error, got %v", err)
	}
}

// An empty registry means nothing is known yet, which must not reject everything.
func TestValidateEventTypesFailsOpenWhenCatalogEmpty(t *testing.T) {
	svc := NewService(nil, WithEventCatalog(catalogOf()))
	if err := svc.validateEventTypes(context.Background(), []string{"payment.captured"}); err != nil {
		t.Fatalf("expected validation to be skipped for an empty catalog, got %v", err)
	}
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
