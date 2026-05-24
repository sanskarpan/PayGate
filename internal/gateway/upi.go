package gateway

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/sanskarpan/PayGate/internal/payment"
)

func (s *Simulator) CreateUPIIntent(_ context.Context, in payment.GatewayUPIIntentCreateInput) (payment.GatewayUPIIntentResult, error) {
	payee := "paygate-" + in.MerchantID + "@upi"
	amount := fmt.Sprintf("%.2f", float64(in.Amount)/100)
	values := url.Values{}
	values.Set("pa", payee)
	values.Set("pn", "PayGate")
	values.Set("tr", in.PaymentID)
	values.Set("tn", "PayGate Order "+in.OrderID)
	values.Set("am", amount)
	values.Set("cu", in.Currency)
	intentURI := "upi://pay?" + values.Encode()
	return payment.GatewayUPIIntentResult{
		GatewayReference: "upi_ref_" + in.PaymentID,
		DeepLink:         intentURI,
		IntentURI:        intentURI,
		ExpiresAt:        in.ExpiresAt,
	}, nil
}

func (s *Simulator) PollUPIIntent(ctx context.Context, in payment.GatewayUPIIntentPollInput) (payment.GatewayUPIIntentStatus, error) {
	scenario := Scenario{
		Mode:        ModeSuccess,
		DelayMS:     0,
		DeclineCode: "UPI_INTENT_FAILED",
	}
	if s.store != nil && in.MerchantID != "" {
		sc, err := s.store.Get(ctx, in.MerchantID)
		if err == nil {
			scenario = sc
		}
	}
	if time.Now().UTC().After(in.ExpiresAt) {
		return payment.GatewayUPIIntentStatus{ProviderStatus: payment.UPIProviderStatusExpired}, nil
	}
	switch scenario.Mode {
	case ModeTimeout:
		return payment.GatewayUPIIntentStatus{ProviderStatus: payment.UPIProviderStatusPending}, nil
	case ModeDecline:
		code := scenario.DeclineCode
		if code == "" {
			code = "UPI_DECLINED"
		}
		return payment.GatewayUPIIntentStatus{
			ProviderStatus:   payment.UPIProviderStatusFailed,
			GatewayReference: in.GatewayReference,
			ErrorCode:        code,
			ErrorDescription: "simulator: upi intent declined",
		}, nil
	case ModeSlow, ModeLateCallback:
		if time.Now().UTC().Before(in.CreatedAt.Add(time.Duration(scenario.DelayMS) * time.Millisecond)) {
			return payment.GatewayUPIIntentStatus{ProviderStatus: payment.UPIProviderStatusPending}, nil
		}
	}
	return payment.GatewayUPIIntentStatus{
		ProviderStatus:   payment.UPIProviderStatusSucceeded,
		GatewayReference: in.GatewayReference,
	}, nil
}
