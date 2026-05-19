package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

var ErrPaymentNotFound = errors.New("payment not found")

type GatewayClient interface {
	Authorize(ctx context.Context, amount int64, currency, merchantID, method string) (GatewayAuthResult, error)
}

type GatewayAuthResult struct {
	Success          bool
	GatewayReference string
	AuthCode         string
	ErrorCode        string
	ErrorDescription string
}

type Service struct {
	repo    Repository
	gateway GatewayClient
}

func NewService(repo Repository, gw GatewayClient) *Service {
	return &Service{repo: repo, gateway: gw}
}

type AuthorizeInput struct {
	PaymentID      string `json:"-"`
	MerchantID     string `json:"-"`
	OrderID        string `json:"order_id"`
	Amount         int64  `json:"amount"`
	Currency       string `json:"currency"`
	Method         string `json:"method"`
	IdempotencyKey string `json:"-"`
	AutoCapture    bool   `json:"auto_capture"`
}

func (s *Service) Authorize(ctx context.Context, in AuthorizeInput) (CaptureResult, error) {
	if in.Amount <= 0 {
		return CaptureResult{}, ErrInvalidPaymentAmount
	}
	if in.PaymentID == "" {
		in.PaymentID = idgen.New("pay")
	}

	pending, err := s.repo.StartAuthorization(ctx, CreateAuthorizedInput{
		PaymentID:      in.PaymentID,
		MerchantID:     in.MerchantID,
		OrderID:        in.OrderID,
		Amount:         in.Amount,
		Currency:       in.Currency,
		Method:         in.Method,
		IdempotencyKey: in.IdempotencyKey,
	})
	if err != nil {
		return CaptureResult{}, err
	}
	if pending.Status != StateCreated {
		return pending, nil
	}

	result, err := s.gateway.Authorize(ctx, in.Amount, in.Currency, in.MerchantID, in.Method)
	if err != nil {
		_ = s.repo.MarkAuthorizationFailed(ctx, in.MerchantID, pending.PaymentID, "GATEWAY_ERROR", err.Error())
		return CaptureResult{}, fmt.Errorf("gateway authorize: %w", err)
	}
	if !result.Success {
		_ = s.repo.MarkAuthorizationFailed(ctx, in.MerchantID, pending.PaymentID, result.ErrorCode, result.ErrorDescription)
		return CaptureResult{}, ErrAuthorizationDeclined
	}

	var autoCaptureAt *time.Time
	if in.AutoCapture {
		t := time.Now().UTC().Add(30 * time.Second)
		autoCaptureAt = &t
	}

	return s.repo.MarkAuthorizationAuthorized(ctx, in.MerchantID, pending.PaymentID, result.GatewayReference, result.AuthCode, autoCaptureAt)
}

func (s *Service) RecordFailedAttempt(ctx context.Context, in AuthorizeInput, errorCode, errorDescription string) error {
	if in.Amount <= 0 {
		return ErrInvalidPaymentAmount
	}
	if in.PaymentID == "" {
		in.PaymentID = idgen.New("pay")
	}
	return s.repo.CreateFailedAttempt(ctx, CreateAuthorizedInput{
		PaymentID:      in.PaymentID,
		MerchantID:     in.MerchantID,
		OrderID:        in.OrderID,
		Amount:         in.Amount,
		Currency:       in.Currency,
		Method:         in.Method,
		IdempotencyKey: in.IdempotencyKey,
	}, errorCode, errorDescription)
}

func (s *Service) CaptureForMerchant(ctx context.Context, merchantID, paymentID string, amount int64) (CaptureResult, error) {
	if merchantID == "" {
		return CaptureResult{}, errors.New("merchant id is required")
	}
	return s.repo.CaptureAuthorizedPayment(ctx, merchantID, paymentID, amount)
}

func (s *Service) ReverseAuthorization(ctx context.Context, merchantID, paymentID, reason string) (CaptureResult, error) {
	return s.repo.ReverseAuthorization(ctx, merchantID, paymentID, reason)
}

func (s *Service) Get(ctx context.Context, merchantID, paymentID string) (CaptureResult, error) {
	return s.repo.GetPayment(ctx, merchantID, paymentID)
}

func (s *Service) List(ctx context.Context, f ListFilter) (ListResult, error) {
	if f.Count <= 0 || f.Count > 100 {
		f.Count = 20
	}
	return s.repo.ListPayments(ctx, f)
}
