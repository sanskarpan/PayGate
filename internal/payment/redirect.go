package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

func (s *Service) CreateRedirectSession(ctx context.Context, in CreateRedirectSessionInput) (RedirectSessionResult, error) {
	if s.redirect == nil {
		return RedirectSessionResult{}, errors.New("redirect gateway is not configured")
	}
	if in.Amount <= 0 {
		return RedirectSessionResult{}, ErrInvalidPaymentAmount
	}
	if in.PaymentID == "" {
		in.PaymentID = idgen.New("pay")
	}
	if in.Method != "netbanking" && in.Method != "wallet" {
		return RedirectSessionResult{}, ErrInvalidTransition
	}
	expiresIn := in.ExpiresInSeconds
	if expiresIn <= 0 {
		expiresIn = 900
	}
	provider := gatewayProviderName(s.gateway)
	decision := GatewayRouteDecision{
		Provider:           provider,
		RoutingReason:      "direct_gateway",
		AttemptedProviders: []string{provider},
		PrimaryProvider:    provider,
	}
	expiresAt := time.Now().UTC().Add(time.Duration(expiresIn) * time.Second)
	callbackToken := idgen.New("redcb")
	session, err := s.repo.CreateRedirectSession(ctx, CreateRedirectSessionRecordInput{
		PaymentID:      in.PaymentID,
		MerchantID:     in.MerchantID,
		OrderID:        in.OrderID,
		Amount:         in.Amount,
		Currency:       in.Currency,
		Method:         in.Method,
		IdempotencyKey: in.IdempotencyKey,
		FlowType:       in.Method,
		BankCode:       in.BankCode,
		WalletCode:     in.WalletCode,
		ExpiresAt:      expiresAt,
		CallbackToken:  callbackToken,
		Provider:       decision.Provider,
		RoutingReason:  decision.RoutingReason,
		Attempted:      decision.AttemptedProviders,
	})
	if err != nil {
		return RedirectSessionResult{}, err
	}
	gatewayResult, err := s.redirect.CreateRedirectSession(ctx, GatewayRedirectCreateInput{
		PaymentID:  session.PaymentID,
		MerchantID: session.MerchantID,
		OrderID:    session.OrderID,
		Amount:     session.Amount,
		Currency:   session.Currency,
		Method:     session.Method,
		BankCode:   session.BankCode,
		WalletCode: session.WalletCode,
		ExpiresAt:  session.ExpiresAt,
	})
	if err != nil {
		_, _, _ = s.repo.FailRedirectSession(ctx, session.PaymentID, "", RedirectProviderStatusFailed, "REDIRECT_GATEWAY_ERROR", err.Error(), time.Now().UTC())
		return RedirectSessionResult{}, fmt.Errorf("create redirect session: %w", err)
	}
	return s.repo.AttachRedirectGatewayData(ctx, session.MerchantID, session.PaymentID, gatewayResult)
}

func (s *Service) GetRedirectSession(ctx context.Context, merchantID, paymentID string) (RedirectSessionResult, error) {
	return s.repo.GetRedirectSession(ctx, merchantID, paymentID)
}

func (s *Service) PollRedirectSession(ctx context.Context, merchantID, paymentID string) (RedirectSessionResult, error) {
	if s.redirect == nil {
		return RedirectSessionResult{}, errors.New("redirect gateway is not configured")
	}
	current, err := s.repo.GetRedirectSession(ctx, merchantID, paymentID)
	if err != nil {
		return RedirectSessionResult{}, err
	}
	if time.Now().UTC().After(current.ExpiresAt) && (current.Status == StatePendingCustomerAction || current.Status == StateProcessing) {
		return s.repo.ExpireRedirectSession(ctx, merchantID, paymentID, time.Now().UTC())
	}
	status, err := s.redirect.PollRedirectSession(ctx, GatewayRedirectPollInput{
		PaymentID:        current.PaymentID,
		MerchantID:       current.MerchantID,
		OrderID:          current.OrderID,
		Method:           current.Method,
		GatewayReference: current.GatewayReference,
		CreatedAt:        current.CreatedAt,
		ExpiresAt:        current.ExpiresAt,
	})
	if err != nil {
		return RedirectSessionResult{}, fmt.Errorf("poll redirect session: %w", err)
	}
	if err := s.repo.PollRedirectSession(ctx, merchantID, paymentID, time.Now().UTC()); err != nil {
		return RedirectSessionResult{}, err
	}
	switch status.ProviderStatus {
	case RedirectProviderStatusPending:
		if current.Status == StatePendingCustomerAction {
			out, _, err := s.repo.MarkRedirectProcessing(ctx, paymentID, "")
			return out, err
		}
		return s.repo.GetRedirectSession(ctx, merchantID, paymentID)
	case RedirectProviderStatusSucceeded:
		out, _, err := s.repo.CompleteRedirectSession(ctx, paymentID, "", status.GatewayReference, time.Now().UTC())
		return out, err
	case RedirectProviderStatusFailed:
		out, _, err := s.repo.FailRedirectSession(ctx, paymentID, "", RedirectProviderStatusFailed, status.ErrorCode, status.ErrorDescription, time.Now().UTC())
		return out, err
	default:
		return s.repo.GetRedirectSession(ctx, merchantID, paymentID)
	}
}

func (s *Service) ApplyRedirectCallback(ctx context.Context, merchantID, paymentID, callbackToken, eventID, status, gatewayReference, errorCode, errorDescription string) (RedirectSessionResult, bool, error) {
	current, err := s.repo.GetRedirectSession(ctx, merchantID, paymentID)
	if err != nil {
		return RedirectSessionResult{}, false, err
	}
	if callbackToken == "" || callbackToken != current.CallbackToken {
		return RedirectSessionResult{}, false, ErrUPICallbackRejected
	}
	now := time.Now().UTC()
	switch status {
	case "processing":
		return s.repo.MarkRedirectProcessing(ctx, paymentID, eventID)
	case "succeeded":
		return s.repo.CompleteRedirectSession(ctx, paymentID, eventID, gatewayReference, now)
	case "failed":
		return s.repo.FailRedirectSession(ctx, paymentID, eventID, RedirectProviderStatusFailed, errorCode, errorDescription, now)
	case "abandoned":
		out, err := s.repo.AbandonRedirectSession(ctx, merchantID, paymentID, errorDescription, now)
		return out, true, err
	case "expired":
		out, err := s.repo.ExpireRedirectSession(ctx, merchantID, paymentID, now)
		return out, true, err
	default:
		return RedirectSessionResult{}, false, ErrUPICallbackRejected
	}
}

func (s *Service) AbandonRedirectSession(ctx context.Context, merchantID, paymentID, reason string) (RedirectSessionResult, error) {
	return s.repo.AbandonRedirectSession(ctx, merchantID, paymentID, reason, time.Now().UTC())
}
