package payment

import (
	"context"
	"testing"
	"time"
)

type fakeRepo struct {
	autoCalled   bool
	expireCalled bool
}

func (f *fakeRepo) StartAuthorization(context.Context, CreateAuthorizedInput) (CaptureResult, error) {
	return CaptureResult{}, nil
}
func (f *fakeRepo) AttachCardPaymentDetails(context.Context, string, string, CardPaymentDetailsInput) error {
	return nil
}
func (f *fakeRepo) UpdateGatewayRouting(context.Context, string, string, GatewayRouteDecision) error {
	return nil
}
func (f *fakeRepo) CreateFailedAttempt(context.Context, CreateAuthorizedInput, string, string) error {
	return nil
}
func (f *fakeRepo) MarkAuthorizationAuthorized(context.Context, string, string, string, string, *time.Time) (CaptureResult, error) {
	return CaptureResult{}, nil
}
func (f *fakeRepo) MarkAuthorizationRequiresAction(context.Context, string, string, CardChallengeSessionInput) (CaptureResult, error) {
	return CaptureResult{}, nil
}
func (f *fakeRepo) MarkAuthorizationFailed(context.Context, string, string, string, string) error {
	return nil
}
func (f *fakeRepo) CompleteCardChallenge(context.Context, string, string, string, string, string, time.Time) (CaptureResult, bool, error) {
	return CaptureResult{}, false, nil
}
func (f *fakeRepo) FailCardChallenge(context.Context, string, string, string, string, string, string, time.Time) (CaptureResult, bool, error) {
	return CaptureResult{}, false, nil
}
func (f *fakeRepo) CreateUPIIntent(context.Context, CreateUPIIntentRecordInput) (UPIIntentResult, error) {
	return UPIIntentResult{}, nil
}
func (f *fakeRepo) AttachUPIIntentGatewayData(context.Context, string, string, GatewayUPIIntentResult) (UPIIntentResult, error) {
	return UPIIntentResult{}, nil
}
func (f *fakeRepo) GetUPIIntent(context.Context, string, string) (UPIIntentResult, error) {
	return UPIIntentResult{}, nil
}
func (f *fakeRepo) PollUPIIntent(context.Context, string, string, time.Time) error {
	return nil
}
func (f *fakeRepo) MarkUPIIntentProcessing(context.Context, string, string) (UPIIntentResult, bool, error) {
	return UPIIntentResult{}, false, nil
}
func (f *fakeRepo) CompleteUPIIntent(context.Context, string, string, string, time.Time) (UPIIntentResult, bool, error) {
	return UPIIntentResult{}, false, nil
}
func (f *fakeRepo) FailUPIIntent(context.Context, string, string, UPIProviderStatus, string, string, time.Time) (UPIIntentResult, bool, error) {
	return UPIIntentResult{}, false, nil
}
func (f *fakeRepo) AbandonUPIIntent(context.Context, string, string, string, time.Time) (UPIIntentResult, error) {
	return UPIIntentResult{}, nil
}
func (f *fakeRepo) ExpireUPIIntent(context.Context, string, string, time.Time) (UPIIntentResult, error) {
	return UPIIntentResult{}, nil
}
func (f *fakeRepo) CreateRedirectSession(context.Context, CreateRedirectSessionRecordInput) (RedirectSessionResult, error) {
	return RedirectSessionResult{}, nil
}
func (f *fakeRepo) AttachRedirectGatewayData(context.Context, string, string, GatewayRedirectResult) (RedirectSessionResult, error) {
	return RedirectSessionResult{}, nil
}
func (f *fakeRepo) GetRedirectSession(context.Context, string, string) (RedirectSessionResult, error) {
	return RedirectSessionResult{}, nil
}
func (f *fakeRepo) PollRedirectSession(context.Context, string, string, time.Time) error {
	return nil
}
func (f *fakeRepo) MarkRedirectProcessing(context.Context, string, string) (RedirectSessionResult, bool, error) {
	return RedirectSessionResult{}, false, nil
}
func (f *fakeRepo) CompleteRedirectSession(context.Context, string, string, string, time.Time) (RedirectSessionResult, bool, error) {
	return RedirectSessionResult{}, false, nil
}
func (f *fakeRepo) FailRedirectSession(context.Context, string, string, RedirectProviderStatus, string, string, time.Time) (RedirectSessionResult, bool, error) {
	return RedirectSessionResult{}, false, nil
}
func (f *fakeRepo) AbandonRedirectSession(context.Context, string, string, string, time.Time) (RedirectSessionResult, error) {
	return RedirectSessionResult{}, nil
}
func (f *fakeRepo) ExpireRedirectSession(context.Context, string, string, time.Time) (RedirectSessionResult, error) {
	return RedirectSessionResult{}, nil
}
func (f *fakeRepo) ReverseAuthorization(context.Context, string, string, string) (CaptureResult, error) {
	return CaptureResult{}, nil
}
func (f *fakeRepo) CaptureAuthorizedPayment(context.Context, string, string, int64) (CaptureResult, error) {
	return CaptureResult{}, nil
}
func (f *fakeRepo) GetPayment(context.Context, string, string) (CaptureResult, error) {
	return CaptureResult{}, nil
}
func (f *fakeRepo) ListPayments(context.Context, ListFilter) (ListResult, error) {
	return ListResult{}, nil
}
func (f *fakeRepo) AutoCaptureDue(context.Context) (int64, error) {
	f.autoCalled = true
	return 0, nil
}
func (f *fakeRepo) ExpireAuthorizationWindow(context.Context, time.Duration) (int64, error) {
	f.expireCalled = true
	return 0, nil
}

func TestSweeperInvokesRepo(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, nil)
	s := NewSweeper(svc, 10*time.Millisecond, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go s.Start(ctx)
	time.Sleep(25 * time.Millisecond)
	cancel()
	if !repo.autoCalled || !repo.expireCalled {
		t.Fatalf("expected sweeper to invoke both paths, auto=%v expire=%v", repo.autoCalled, repo.expireCalled)
	}
}
