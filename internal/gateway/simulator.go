package gateway

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
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
	store        *ScenarioStore     // nil = use env var / ForceDecline behaviour
	methodStore  *MethodConfigStore // nil = use default method profiles only
}

func (s *Simulator) Name() string {
	return "simulator"
}

// effectiveSuccessRate applies the GATEWAY_SIM_SUCCESS_RATE override when set.
//
// The simulator declines a small share of authorizations by default so the
// sandbox behaves like a real acquirer. That randomness is unhelpful for
// automated suites and demos, which need a deterministic gateway, so the rate
// can be pinned the same way GATEWAY_SIM_FORCE_DECLINE pins the outcome.
// Values outside [0,1] are ignored.
func effectiveSuccessRate(rate float64) float64 {
	raw := strings.TrimSpace(os.Getenv("GATEWAY_SIM_SUCCESS_RATE"))
	if raw == "" {
		return rate
	}
	override, err := strconv.ParseFloat(raw, 64)
	// NaN parses successfully and fails every comparison, so a bare range check
	// would let it through and silently pin the gateway to always succeeding.
	if err != nil || math.IsNaN(override) || override < 0 || override > 1 {
		return rate
	}
	return override
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

// NewSimulatorWithStores creates a Simulator backed by both a ScenarioStore and a MethodConfigStore.
func NewSimulatorWithStores(scenarioStore *ScenarioStore, methodStore *MethodConfigStore) *Simulator {
	declineCode := os.Getenv("GATEWAY_SIM_DECLINE_CODE")
	if declineCode == "" {
		declineCode = "CARD_DECLINED"
	}
	return &Simulator{
		ForceDecline: os.Getenv("GATEWAY_SIM_FORCE_DECLINE") == "true",
		DeclineCode:  declineCode,
		store:        scenarioStore,
		methodStore:  methodStore,
	}
}

// Authorize simulates a gateway authorization.
// When a ScenarioStore is configured and merchantID is non-empty the active
// scenario for that merchant is applied; otherwise the legacy ForceDecline
// behaviour is used. The method parameter drives method-specific behaviour.
func (s *Simulator) Authorize(ctx context.Context, amount int64, currency, merchantID, method string) (payment.GatewayAuthResult, error) {
	return s.AuthorizeWithProvider(ctx, ProviderSimulatorPrimary, amount, currency, merchantID, method)
}

func (s *Simulator) AuthorizeWithProvider(ctx context.Context, provider string, amount int64, currency, merchantID, method string) (payment.GatewayAuthResult, error) {
	if provider == "" {
		provider = ProviderSimulatorPrimary
	}
	if provider == ProviderSimulatorFailover {
		profile := ProfileForMethod(method)
		time.Sleep(time.Duration(profile.DefaultDelayMS) * time.Millisecond)
		return stampProvider(provider, payment.GatewayAuthResult{Success: true, GatewayReference: "gw_ref_success", AuthCode: "AUTH_OK"}), nil
	}
	if provider == ProviderSimulatorSaver {
		profile := ProfileForMethod(method)
		time.Sleep(time.Duration(profile.DefaultDelayMS) * time.Millisecond)
		if rand.Float64() >= maxFloat(0.70, profile.SuccessRate-0.08) { //nolint:gosec // simulator uses non-crypto RNG intentionally
			return stampProvider(provider, payment.GatewayAuthResult{
				Success:          false,
				ErrorCode:        methodDeclineCode(profile),
				ErrorDescription: "simulator: cost-saver route decline",
			}), nil
		}
		return stampProvider(provider, payment.GatewayAuthResult{Success: true, GatewayReference: "gw_ref_success", AuthCode: "AUTH_OK"}), nil
	}
	if s.store != nil && merchantID != "" {
		sc, err := s.store.Get(ctx, merchantID)
		if err != nil {
			// Fall through to legacy behaviour on store error.
			res, authErr := s.legacyAuthorize()
			return stampProvider(provider, res), authErr
		}
		result, err := s.applyScenario(ctx, sc, method)
		return stampProvider(provider, result), err
	}
	res, authErr := s.legacyAuthorize()
	return stampProvider(provider, res), authErr
}

// RefundAuthorize simulates an instant gateway refund approval.
func (s *Simulator) RefundAuthorize(_ context.Context, _, _ string, _ int64) (bool, error) {
	return true, nil
}

func (s *Simulator) legacyAuthorize() (payment.GatewayAuthResult, error) {
	profile := ProfileForMethod("card")
	time.Sleep(time.Duration(profile.DefaultDelayMS) * time.Millisecond)
	if s.ForceDecline {
		return payment.GatewayAuthResult{
			Success:          false,
			ErrorCode:        s.DeclineCode,
			ErrorDescription: "simulator: authorization declined",
		}, nil
	}
	return payment.GatewayAuthResult{Success: true, GatewayReference: "gw_ref_success", AuthCode: "AUTH_OK"}, nil
}

func stampProvider(provider string, result payment.GatewayAuthResult) payment.GatewayAuthResult {
	if provider == "" {
		return result
	}
	if result.GatewayReference != "" {
		result.GatewayReference = provider + ":" + result.GatewayReference
	}
	if result.AuthCode != "" {
		result.AuthCode = provider + ":" + result.AuthCode
	}
	if result.ChallengeToken != "" {
		result.ChallengeToken = provider + ":" + result.ChallengeToken
	}
	if result.ChallengeURL != "" {
		result.ChallengeURL = result.ChallengeURL + "?provider=" + provider
	}
	return result
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// methodDeclineCode picks the first decline code from the profile, or a generic fallback.
func methodDeclineCode(profile MethodProfile) string {
	if len(profile.DeclineCodes) > 0 {
		return profile.DeclineCodes[0]
	}
	return "DECLINED"
}

func (s *Simulator) applyScenario(ctx context.Context, sc Scenario, method string) (payment.GatewayAuthResult, error) {
	profile := ProfileForMethod(method)

	// Resolve method config override if a methodStore is available.
	var override *MethodConfig
	if s.methodStore != nil {
		cfg, err := s.methodStore.Get(ctx, sc.MerchantID, method)
		if err == nil {
			override = &cfg
		} else if !errors.Is(err, ErrMethodConfigNotFound) {
			// Non-fatal: log and proceed with defaults.
			_ = err
		}
	}

	// If the method is disabled via override, decline immediately.
	if override != nil && !override.Enabled {
		decCode := override.DeclineCode
		if decCode == "" {
			decCode = methodDeclineCode(profile)
		}
		return payment.GatewayAuthResult{
			Success:          false,
			ErrorCode:        decCode,
			ErrorDescription: "simulator: method disabled",
		}, nil
	}

	switch sc.Mode {
	case ModeSuccess:
		delayMS := profile.DefaultDelayMS
		if override != nil && override.DelayMS > delayMS {
			delayMS = override.DelayMS
		}
		time.Sleep(time.Duration(delayMS) * time.Millisecond)

		// Determine effective success rate.
		successRate := effectiveSuccessRate(profile.SuccessRate)
		decCode := methodDeclineCode(profile)
		if override != nil {
			successRate = effectiveSuccessRate(override.SuccessRate)
			if override.DeclineCode != "" {
				decCode = override.DeclineCode
			}
		}
		if rand.Float64() >= successRate { //nolint:gosec // simulator uses non-crypto RNG intentionally
			return payment.GatewayAuthResult{
				Success:          false,
				ErrorCode:        decCode,
				ErrorDescription: "simulator: method rate decline",
			}, nil
		}
		return payment.GatewayAuthResult{Success: true, GatewayReference: "gw_ref_success", AuthCode: "AUTH_OK"}, nil

	case ModeSlow, ModeLateCallback:
		time.Sleep(time.Duration(sc.DelayMS) * time.Millisecond)
		return payment.GatewayAuthResult{Success: true, GatewayReference: "gw_ref_success", AuthCode: "AUTH_OK"}, nil

	case ModeFlaky:
		time.Sleep(time.Duration(profile.DefaultDelayMS) * time.Millisecond)
		if rand.Float64() < sc.FailureRate { //nolint:gosec // simulator uses non-crypto RNG intentionally
			decCode := sc.DeclineCode
			if decCode == "" {
				decCode = methodDeclineCode(profile)
			}
			return payment.GatewayAuthResult{
				Success:          false,
				ErrorCode:        decCode,
				ErrorDescription: "simulator: flaky decline",
			}, nil
		}
		// Scenario passed; now apply method success rate.
		successRate := effectiveSuccessRate(profile.SuccessRate)
		decCode := methodDeclineCode(profile)
		if override != nil {
			successRate = effectiveSuccessRate(override.SuccessRate)
			if override.DeclineCode != "" {
				decCode = override.DeclineCode
			}
		}
		if rand.Float64() >= successRate { //nolint:gosec // simulator uses non-crypto RNG intentionally
			return payment.GatewayAuthResult{
				Success:          false,
				ErrorCode:        decCode,
				ErrorDescription: "simulator: method rate decline",
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
		time.Sleep(time.Duration(profile.DefaultDelayMS) * time.Millisecond)
		decCode := sc.DeclineCode
		if decCode == "" {
			decCode = methodDeclineCode(profile)
		}
		return payment.GatewayAuthResult{
			Success:          false,
			ErrorCode:        decCode,
			ErrorDescription: "simulator: authorization declined",
		}, nil

	case ModeChallenge:
		time.Sleep(time.Duration(profile.DefaultDelayMS) * time.Millisecond)
		expiresAt := time.Now().UTC().Add(5 * time.Minute)
		return payment.GatewayAuthResult{
			RequiresAction:   true,
			GatewayReference: "gw_ref_3ds_pending",
			ChallengeURL:     "https://sandbox.paygate.local/3ds/" + sc.MerchantID,
			ChallengeToken:   "3ds_tok_" + sc.MerchantID,
			ChallengeExpiry:  &expiresAt,
		}, nil

	default:
		time.Sleep(time.Duration(profile.DefaultDelayMS) * time.Millisecond)
		return payment.GatewayAuthResult{Success: true, GatewayReference: "gw_ref_success", AuthCode: "AUTH_OK"}, nil
	}
}
