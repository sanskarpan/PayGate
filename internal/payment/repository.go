package payment

import (
	"context"
	"time"
)

type CaptureResult struct {
	PaymentID    string
	MerchantID   string
	OrderID      string
	Amount       int64
	Currency     string
	Method       string
	Status       PaymentState
	Captured     bool
	CapturedAt   *time.Time
	CreatedAt    time.Time
	AuthorizedAt *time.Time
}

type Repository interface {
	StartAuthorization(ctx context.Context, in CreateAuthorizedInput) (CaptureResult, error)
	CreateFailedAttempt(ctx context.Context, in CreateAuthorizedInput, errorCode, errorDescription string) error
	MarkAuthorizationAuthorized(ctx context.Context, merchantID, paymentID, gatewayReference, authCode string, autoCaptureAt *time.Time) (CaptureResult, error)
	MarkAuthorizationFailed(ctx context.Context, merchantID, paymentID, errorCode, errorDescription string) error
	ReverseAuthorization(ctx context.Context, merchantID, paymentID, reason string) (CaptureResult, error)
	CaptureAuthorizedPayment(ctx context.Context, merchantID, paymentID string, amount int64) (CaptureResult, error)
	GetPayment(ctx context.Context, merchantID, paymentID string) (CaptureResult, error)
	ListPayments(ctx context.Context, f ListFilter) (ListResult, error)
	AutoCaptureDue(ctx context.Context) (int64, error)
	ExpireAuthorizationWindow(ctx context.Context, window time.Duration) (int64, error)
}

type ListFilter struct {
	MerchantID string
	OrderID    string
	Count      int
}

type ListResult struct {
	Items []CaptureResult
}

type CreateAuthorizedInput struct {
	PaymentID        string
	MerchantID       string
	OrderID          string
	Amount           int64
	Currency         string
	Method           string
	IdempotencyKey   string
	GatewayReference string
	AuthCode         string
	AutoCaptureAt    *time.Time
}
