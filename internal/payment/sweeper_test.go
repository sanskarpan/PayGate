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

func (f *fakeRepo) CreateAuthorizedPayment(context.Context, CreateAuthorizedInput) (CaptureResult, error) {
	return CaptureResult{}, nil
}
func (f *fakeRepo) CreateFailedAttempt(context.Context, CreateAuthorizedInput, string, string) error {
	return nil
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
