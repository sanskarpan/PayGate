package gateway

import (
	"context"
	"time"

	"github.com/sanskarpan/PayGate/internal/payment"
)

type Simulator struct{}

func NewSimulator() *Simulator {
	return &Simulator{}
}

func (s *Simulator) Authorize(_ context.Context, _ int64, _ string, _ string) (payment.GatewayAuthResult, error) {
	time.Sleep(50 * time.Millisecond)
	return payment.GatewayAuthResult{Success: true, GatewayReference: "gw_ref_success", AuthCode: "AUTH_OK"}, nil
}
