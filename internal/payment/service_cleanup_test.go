package payment

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sanskarpan/PayGate/internal/tokenization"
)

type fakeCardTokenAuthorizer struct {
	completed bool
	err       error
}

func (f *fakeCardTokenAuthorizer) PrepareAuthorization(context.Context, string, string, string) (tokenization.CardTokenReference, error) {
	return tokenization.CardTokenReference{}, errors.New("not implemented")
}

func (f *fakeCardTokenAuthorizer) CompleteAuthorization(context.Context, string, string, string, bool, string) error {
	f.completed = true
	return f.err
}

func TestPersistAuthorizationFailureJoinsCleanupErrors(t *testing.T) {
	repo := &fakeRepo{
		updateRoutingErr:     errors.New("routing write failed"),
		markAuthorizationErr: errors.New("auth state write failed"),
	}
	cardTokens := &fakeCardTokenAuthorizer{err: errors.New("token release failed")}
	svc := NewService(repo, nil, WithCardTokenAuthorizer(cardTokens))

	err := svc.persistAuthorizationFailure(context.Background(), CaptureResult{
		PaymentID:            "pay_1",
		MerchantID:           "merch_1",
		Method:               "card",
		PaymentMethodTokenID: "tok_1",
	}, GatewayRouteDecision{Provider: "sim"}, "GATEWAY_ERROR", "gateway exploded", "gateway error", errors.New("gateway exploded"))
	if err == nil {
		t.Fatal("expected joined error")
	}
	if !repo.updatedRouting || !repo.markedAuthFailed || !cardTokens.completed {
		t.Fatalf("expected all cleanup steps to run, got routing=%v mark_failed=%v token=%v", repo.updatedRouting, repo.markedAuthFailed, cardTokens.completed)
	}
	for _, want := range []string{"gateway exploded", "routing write failed", "token release failed", "auth state write failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
}

func TestPersistUPIIntentFailureReturnsCleanupError(t *testing.T) {
	repo := &fakeRepo{failUPIIntentErr: errors.New("upi fail persist failed")}
	svc := NewService(repo, nil)

	err := svc.persistUPIIntentFailure(context.Background(), "pay_1", "UPI_GATEWAY_ERROR", "gateway exploded", errors.New("gateway exploded"))
	if err == nil {
		t.Fatal("expected joined error")
	}
	if !repo.failedUPIIntent {
		t.Fatal("expected failure-state persistence to be attempted")
	}
	for _, want := range []string{"gateway exploded", "upi fail persist failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
}
