package gateway

import (
	"context"
	"os"
	"time"

	"github.com/sanskarpan/PayGate/internal/payment"
)

// Simulator is a deterministic payment gateway simulator.
// Set ForceDecline=true (or GATEWAY_SIM_FORCE_DECLINE=true env var) to make
// all authorization attempts fail — useful for testing failure-path code paths.
type Simulator struct {
	ForceDecline bool
	DeclineCode  string
}

func NewSimulator() *Simulator {
	forceDecline := os.Getenv("GATEWAY_SIM_FORCE_DECLINE") == "true"
	declineCode := os.Getenv("GATEWAY_SIM_DECLINE_CODE")
	if declineCode == "" {
		declineCode = "CARD_DECLINED"
	}
	return &Simulator{ForceDecline: forceDecline, DeclineCode: declineCode}
}

func NewSimulatorWithOptions(forceDecline bool, declineCode string) *Simulator {
	if declineCode == "" {
		declineCode = "CARD_DECLINED"
	}
	return &Simulator{ForceDecline: forceDecline, DeclineCode: declineCode}
}

func (s *Simulator) Authorize(_ context.Context, _ int64, _ string, _ string) (payment.GatewayAuthResult, error) {
	time.Sleep(50 * time.Millisecond)
	if s.ForceDecline {
		return payment.GatewayAuthResult{
			Success:          false,
			ErrorCode:        s.DeclineCode,
			ErrorDescription: "simulator: authorization declined",
		}, nil
	}
	return payment.GatewayAuthResult{Success: true, GatewayReference: "gw_ref_success", AuthCode: "AUTH_OK"}, nil
}
