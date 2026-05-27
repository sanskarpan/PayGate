package gateway

import (
	"context"
	"time"

	"github.com/sanskarpan/PayGate/internal/payment"
)

func (s *Simulator) CreateRedirectSession(_ context.Context, in payment.GatewayRedirectCreateInput) (payment.GatewayRedirectResult, error) {
	if in.ExpiresAt.IsZero() {
		in.ExpiresAt = time.Now().UTC().Add(15 * time.Minute)
	}
	result := payment.GatewayRedirectResult{
		GatewayReference: "redir_ref_" + in.Method,
		RedirectURL:      "https://sandbox.paygate.local/" + in.Method + "/" + in.PaymentID,
		BankCode:         in.BankCode,
		WalletCode:       in.WalletCode,
		ExpiresAt:        in.ExpiresAt,
	}
	if in.Method == "netbanking" {
		if result.BankCode == "" {
			result.BankCode = "HDFC"
		}
		result.BankName = bankLabel(result.BankCode)
	}
	if in.Method == "wallet" {
		if result.WalletCode == "" {
			result.WalletCode = "paytm"
		}
		result.WalletName = walletLabel(result.WalletCode)
	}
	return result, nil
}

func (s *Simulator) PollRedirectSession(ctx context.Context, in payment.GatewayRedirectPollInput) (payment.GatewayRedirectStatus, error) {
	if s.store != nil && in.MerchantID != "" {
		sc, err := s.store.Get(ctx, in.MerchantID)
		if err == nil {
			switch sc.Mode {
			case ModeDecline:
				return payment.GatewayRedirectStatus{
					ProviderStatus:   payment.RedirectProviderStatusFailed,
					GatewayReference: in.GatewayReference,
					ErrorCode:        methodDeclineCode(ProfileForMethod(in.Method)),
					ErrorDescription: "simulator: redirect payment failed",
				}, nil
			case ModeTimeout:
				return payment.GatewayRedirectStatus{ProviderStatus: payment.RedirectProviderStatusPending, GatewayReference: in.GatewayReference}, nil
			case ModeSuccess, ModeSlow, ModeLateCallback:
				return payment.GatewayRedirectStatus{ProviderStatus: payment.RedirectProviderStatusSucceeded, GatewayReference: in.GatewayReference}, nil
			}
		}
	}
	if time.Now().UTC().After(in.ExpiresAt) {
		return payment.GatewayRedirectStatus{ProviderStatus: payment.RedirectProviderStatusExpired, GatewayReference: in.GatewayReference}, nil
	}
	return payment.GatewayRedirectStatus{ProviderStatus: payment.RedirectProviderStatusPending, GatewayReference: in.GatewayReference}, nil
}

func bankLabel(code string) string {
	switch code {
	case "HDFC":
		return "HDFC Bank"
	case "ICICI":
		return "ICICI Bank"
	case "SBI":
		return "State Bank of India"
	default:
		return code
	}
}

func walletLabel(code string) string {
	switch code {
	case "phonepe":
		return "PhonePe"
	case "mobikwik":
		return "MobiKwik"
	case "paytm":
		return "Paytm Wallet"
	default:
		return code
	}
}
