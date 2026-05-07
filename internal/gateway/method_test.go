package gateway

import (
	"context"
	"testing"

	"github.com/sanskarpan/PayGate/internal/payment"
)

// TestProfileForMethod verifies that each known method returns the correct
// default profile, and that an unknown method falls back to card defaults.
func TestProfileForMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method          string
		wantDelayMS     int
		wantSuccessRate float64
		wantFirstCode   string
	}{
		{
			method:          "card",
			wantDelayMS:     50,
			wantSuccessRate: 0.97,
			wantFirstCode:   "INSUFFICIENT_FUNDS",
		},
		{
			method:          "upi",
			wantDelayMS:     30,
			wantSuccessRate: 0.95,
			wantFirstCode:   "VPA_INVALID",
		},
		{
			method:          "netbanking",
			wantDelayMS:     150,
			wantSuccessRate: 0.92,
			wantFirstCode:   "NET_BANKING_FAILED",
		},
		{
			method:          "wallet",
			wantDelayMS:     20,
			wantSuccessRate: 0.99,
			wantFirstCode:   "WALLET_INSUFFICIENT_BALANCE",
		},
		{
			// Unknown method must fall back to card defaults.
			method:          "unknown_method",
			wantDelayMS:     50,
			wantSuccessRate: 0.97,
			wantFirstCode:   "INSUFFICIENT_FUNDS",
		},
	}

	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			t.Parallel()
			p := ProfileForMethod(tc.method)
			if p.DefaultDelayMS != tc.wantDelayMS {
				t.Errorf("method %q: DefaultDelayMS = %d, want %d", tc.method, p.DefaultDelayMS, tc.wantDelayMS)
			}
			if p.SuccessRate != tc.wantSuccessRate {
				t.Errorf("method %q: SuccessRate = %f, want %f", tc.method, p.SuccessRate, tc.wantSuccessRate)
			}
			if len(p.DeclineCodes) == 0 {
				t.Fatalf("method %q: DeclineCodes is empty", tc.method)
			}
			if p.DeclineCodes[0] != tc.wantFirstCode {
				t.Errorf("method %q: DeclineCodes[0] = %q, want %q", tc.method, p.DeclineCodes[0], tc.wantFirstCode)
			}
		})
	}
}

// methodBehaviorHarness wires a Simulator directly with a known scenario (bypasses DB).
// It calls applyScenario directly so we can test method behavior deterministically.
type methodBehaviorHarness struct {
	sim      *Simulator
	scenario Scenario
}

func newHarness(mode ScenarioMode, declineCode string) *methodBehaviorHarness {
	sc := Scenario{
		MerchantID:  "test_merchant",
		Mode:        mode,
		FailureRate: 1.0, // always fail when flaky
		DeclineCode: declineCode,
		Active:      true,
	}
	sim := &Simulator{
		ForceDecline: false,
		DeclineCode:  "CARD_DECLINED",
	}
	return &methodBehaviorHarness{sim: sim, scenario: sc}
}

func (h *methodBehaviorHarness) authorize(method string) (payment.GatewayAuthResult, error) {
	return h.sim.applyScenario(context.Background(), h.scenario, method)
}

// TestSimulator_MethodBehavior verifies that the decline code returned when
// mode=ModeDecline reflects the method-appropriate code (scenario.DeclineCode
// wins when set; the method profile first-code is used otherwise).
func TestSimulator_MethodBehavior(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mode        ScenarioMode
		declineCode string // scenario's configured decline code
		method      string
		wantSuccess bool
		wantCode    string // only checked when !wantSuccess
	}{
		{
			name:        "card decline returns scenario code",
			mode:        ModeDecline,
			declineCode: "CARD_DECLINED",
			method:      "card",
			wantSuccess: false,
			wantCode:    "CARD_DECLINED",
		},
		{
			name:        "upi decline returns scenario code",
			mode:        ModeDecline,
			declineCode: "VPA_INVALID",
			method:      "upi",
			wantSuccess: false,
			wantCode:    "VPA_INVALID",
		},
		{
			name:        "netbanking decline returns scenario code",
			mode:        ModeDecline,
			declineCode: "NET_BANKING_FAILED",
			method:      "netbanking",
			wantSuccess: false,
			wantCode:    "NET_BANKING_FAILED",
		},
		{
			name:        "wallet decline returns scenario code",
			mode:        ModeDecline,
			declineCode: "WALLET_BLOCKED",
			method:      "wallet",
			wantSuccess: false,
			wantCode:    "WALLET_BLOCKED",
		},
		{
			name:        "upi decline with empty scenario code falls back to profile",
			mode:        ModeDecline,
			declineCode: "",
			method:      "upi",
			wantSuccess: false,
			wantCode:    "VPA_INVALID",
		},
		{
			name:        "netbanking decline with empty scenario code falls back to profile",
			mode:        ModeDecline,
			declineCode: "",
			method:      "netbanking",
			wantSuccess: false,
			wantCode:    "NET_BANKING_FAILED",
		},
		{
			name:        "wallet decline with empty scenario code falls back to profile",
			mode:        ModeDecline,
			declineCode: "",
			method:      "wallet",
			wantSuccess: false,
			wantCode:    "WALLET_INSUFFICIENT_BALANCE",
		},
		{
			name:        "card decline with empty scenario code falls back to profile",
			mode:        ModeDecline,
			declineCode: "",
			method:      "card",
			wantSuccess: false,
			wantCode:    "INSUFFICIENT_FUNDS",
		},
		// Flaky with FailureRate=1.0 always declines.
		{
			name:        "upi flaky always declines with upi code",
			mode:        ModeFlaky,
			declineCode: "",
			method:      "upi",
			wantSuccess: false,
			wantCode:    "VPA_INVALID",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(tc.mode, tc.declineCode)
			result, err := h.authorize(tc.method)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Success != tc.wantSuccess {
				t.Errorf("Success = %v, want %v", result.Success, tc.wantSuccess)
			}
			if !tc.wantSuccess && result.ErrorCode != tc.wantCode {
				t.Errorf("ErrorCode = %q, want %q", result.ErrorCode, tc.wantCode)
			}
		})
	}
}
