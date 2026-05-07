package gateway

import (
	"context"
	"math/rand"
	"os"
	"time"

	"github.com/sanskarpan/PayGate/internal/payment"
)

// Simulator is a deterministic payment gateway simulator.
// Set ForceDecline=true (or GATEWAY_SIM_FORCE_DECLINE=true env var) to make
// all authorization attempts fail — useful for testing failure-path code paths.
// When store is non-nil, the active scenario for the merchant is used instead.
type Simulator struct {
	ForceDecline bool
	DeclineCode  string
	store        *ScenarioStore // nil = use env var / ForceDecline behaviour
}

// NewSimulator creates a Simulator driven by environment variables.
func NewSimulator() *Simulator {
	forceDecline := os.Getenv("GATEWAY_SIM_FORCE_DECLINE") == "true"
	declineCode := os.Getenv("GATEWAY_SIM_DECLINE_CODE")
	if declineCode == "" {
		declineCode = "CARD_DECLINED"
	}
	return &Simulator{ForceDecline: forceDecline, DeclineCode: declineCode}
}

// NewSimulatorWithOptions creates a Simulator with explicit options (no store).
func NewSimulatorWithOptions(forceDecline bool, declineCode string) *Simulator {
	if declineCode == "" {
		declineCode = "CARD_DECLINED"
	}
	return &Simulator{ForceDecline: forceDecline, DeclineCode: declineCode}
}

// NewSimulatorWithStore creates a Simulator backed by a ScenarioStore.
func NewSimulatorWithStore(store *ScenarioStore) *Simulator {
	declineCode := os.Getenv("GATEWAY_SIM_DECLINE_CODE")
	if declineCode == "" {
		declineCode = "CARD_DECLINED"
	}
	return &Simulator{
		ForceDecline: os.Getenv("GATEWAY_SIM_FORCE_DECLINE") == "true",
		DeclineCode:  declineCode,
		store:        store,
	}
}

// Authorize simulates a gateway authorization.
// When a ScenarioStore is configured and merchantID is non-empty the active
// scenario for that merchant is applied; otherwise the legacy ForceDecline
// behaviour is used.
func (s *Simulator) Authorize(ctx context.Context, amount int64, currency, merchantID string) (payment.GatewayAuthResult, error) {
	if s.store != nil && merchantID != "" {
		sc, err := s.store.Get(ctx, merchantID)
		if err != nil {
			// Fall through to legacy behaviour on store error.
			return s.legacyAuthorize()
		}
		return s.applyScenario(ctx, sc)
	}
	return s.legacyAuthorize()
}

// RefundAuthorize simulates an instant gateway refund approval.
func (s *Simulator) RefundAuthorize(_ context.Context, _, _ string, _ int64) (bool, error) {
	return true, nil
}

func (s *Simulator) legacyAuthorize() (payment.GatewayAuthResult, error) {
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

func (s *Simulator) applyScenario(ctx context.Context, sc Scenario) (payment.GatewayAuthResult, error) {
	switch sc.Mode {
	case ModeSuccess:
		time.Sleep(50 * time.Millisecond)
		return payment.GatewayAuthResult{Success: true, GatewayReference: "gw_ref_success", AuthCode: "AUTH_OK"}, nil

	case ModeSlow, ModeLateCallback:
		time.Sleep(time.Duration(sc.DelayMS) * time.Millisecond)
		return payment.GatewayAuthResult{Success: true, GatewayReference: "gw_ref_success", AuthCode: "AUTH_OK"}, nil

	case ModeFlaky:
		time.Sleep(50 * time.Millisecond)
		if rand.Float64() < sc.FailureRate {
			return payment.GatewayAuthResult{
				Success:          false,
				ErrorCode:        sc.DeclineCode,
				ErrorDescription: "simulator: flaky decline",
			}, nil
		}
		return payment.GatewayAuthResult{Success: true, GatewayReference: "gw_ref_success", AuthCode: "AUTH_OK"}, nil

	case ModeTimeout:
		select {
		case <-ctx.Done():
			return payment.GatewayAuthResult{}, ctx.Err()
		case <-time.After(30 * time.Second):
			return payment.GatewayAuthResult{}, context.DeadlineExceeded
		}

	case ModeDecline:
		time.Sleep(50 * time.Millisecond)
		return payment.GatewayAuthResult{
			Success:          false,
			ErrorCode:        sc.DeclineCode,
			ErrorDescription: "simulator: authorization declined",
		}, nil

	default:
		time.Sleep(50 * time.Millisecond)
		return payment.GatewayAuthResult{Success: true, GatewayReference: "gw_ref_success", AuthCode: "AUTH_OK"}, nil
	}
}
