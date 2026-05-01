package refund

import "context"

// Service orchestrates refund use-cases.
// For the simulated gateway, ProcessRefund is called immediately after
// CreateRefund — the simulator always succeeds.
type Service struct {
	repo Repository
}

// NewService creates a new Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Initiate creates a new refund and immediately processes it via the simulator.
// Returns the processed Refund on success.
func (s *Service) Initiate(ctx context.Context, in CreateInput) (Refund, error) {
	ref, err := s.repo.CreateRefund(ctx, in)
	if err != nil {
		return Refund{}, err
	}
	// Simulator always succeeds; call ProcessRefund synchronously.
	return s.repo.ProcessRefund(ctx, ref.ID)
}

// Get returns a single refund scoped to the merchant.
func (s *Service) Get(ctx context.Context, merchantID, refundID string) (Refund, error) {
	return s.repo.GetRefund(ctx, merchantID, refundID)
}

// ListByPayment returns all refunds for a payment, scoped to the merchant.
func (s *Service) ListByPayment(ctx context.Context, merchantID, paymentID string) ([]Refund, error) {
	return s.repo.ListRefunds(ctx, merchantID, paymentID)
}
