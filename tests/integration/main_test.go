//go:build integration

package integration

import (
	"os"
	"testing"
)

// TestMain pins the gateway simulator to a deterministic success rate.
//
// The simulator declines a small share of authorizations by default (3% for
// card) so the sandbox behaves like a real acquirer. Integration tests assert
// business logic, not the simulator's dice, and a random decline in an
// unrelated setup step fails the whole test. Tests that want a failure use an
// explicit gateway scenario instead.
func TestMain(m *testing.M) {
	if os.Getenv("GATEWAY_SIM_SUCCESS_RATE") == "" {
		_ = os.Setenv("GATEWAY_SIM_SUCCESS_RATE", "1")
	}
	os.Exit(m.Run())
}
