package gateway

import "testing"

func TestEffectiveSuccessRateHonoursValidOverrides(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want float64
	}{
		{"1", 1},
		{"0", 0},
		{"0.5", 0.5},
		{" 0.75 ", 0.75}, // surrounding whitespace is tolerated
	} {
		t.Setenv("GATEWAY_SIM_SUCCESS_RATE", tc.raw)
		if got := effectiveSuccessRate(0.97); got != tc.want {
			t.Fatalf("raw %q: expected %v, got %v", tc.raw, tc.want, got)
		}
	}
}

// Anything unparseable or out of range must leave the configured rate alone.
// NaN matters specifically: it parses successfully and fails every comparison,
// so a bare range check would let it through and pin the gateway to always
// succeeding.
func TestEffectiveSuccessRateIgnoresInvalidOverrides(t *testing.T) {
	const configured = 0.97
	for _, raw := range []string{
		"", "abc", "1.1", "-0.1", "2", "-5",
		"NaN", "nan", "-NaN", "+Inf", "-Inf", "Inf",
	} {
		t.Setenv("GATEWAY_SIM_SUCCESS_RATE", raw)
		if got := effectiveSuccessRate(configured); got != configured {
			t.Fatalf("raw %q should have been ignored, got %v", raw, got)
		}
	}
}

func TestEffectiveSuccessRateUnsetLeavesRateUnchanged(t *testing.T) {
	t.Setenv("GATEWAY_SIM_SUCCESS_RATE", "")
	if got := effectiveSuccessRate(0.92); got != 0.92 {
		t.Fatalf("expected the configured rate to be preserved, got %v", got)
	}
}
