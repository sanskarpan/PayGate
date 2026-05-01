package risk

import (
	"context"
	"log/slog"
)

// Service evaluates payment risk and manages risk event records.
type Service struct {
	repo    Repository
	logger  *slog.Logger
	alertFn AlertFunc
}

// WithAlertFunc returns a functional option that sets the alert function on the Service.
func WithAlertFunc(fn AlertFunc) func(*Service) {
	return func(s *Service) {
		s.alertFn = fn
	}
}

func NewService(repo Repository, logger *slog.Logger, opts ...func(*Service)) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	svc := &Service{repo: repo, logger: logger}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// EvaluatePayment performs a risk evaluation for a payment attempt.
// It:
//  1. Increments velocity counters for merchant_id and IP address.
//  2. Fetches merchant average transaction amount.
//  3. Calls Evaluate() to compute score and action.
//  4. Persists a RiskEvent and returns it.
//
// Errors in velocity counting are non-fatal — the evaluation continues
// with whatever data is available.
func (s *Service) EvaluatePayment(ctx context.Context, in EvalInput) (RiskEvent, error) {
	merchantHourly, err := s.repo.UpsertVelocityCounter(ctx, "merchant_id", in.MerchantID, VelocityWindow1H, in.Amount)
	if err != nil {
		s.logger.Warn("upsert merchant velocity counter failed", "error", err)
		merchantHourly = 0
	}

	ipHourly := 0
	if in.IPAddress != "" {
		ipHourly, err = s.repo.UpsertVelocityCounter(ctx, "ip_address", in.IPAddress, VelocityWindow1H, in.Amount)
		if err != nil {
			s.logger.Warn("upsert ip velocity counter failed", "error", err)
			ipHourly = 0
		}
	}

	if in.MerchantAvgTxn == 0 {
		avg, err := s.repo.MerchantAverageTxnAmount(ctx, in.MerchantID)
		if err != nil {
			s.logger.Warn("fetch merchant avg txn failed", "error", err)
		}
		in.MerchantAvgTxn = avg
	}

	result := Evaluate(in, merchantHourly, ipHourly)

	ev, err := s.repo.CreateRiskEvent(ctx, RiskEvent{
		MerchantID:     in.MerchantID,
		PaymentID:      in.PaymentID,
		Score:          result.Score,
		Action:         result.Action,
		TriggeredRules: result.TriggeredRules,
	})
	if err != nil {
		return RiskEvent{}, err
	}

	if result.Action != RiskActionAllow {
		s.logger.Info("risk hold/block",
			"payment_id", in.PaymentID,
			"score", result.Score,
			"action", result.Action,
			"rules", result.TriggeredRules,
		)
		if s.alertFn != nil {
			go s.alertFn(ctx, ev)
		}
	}

	return ev, nil
}

// GetRiskEvent returns a single risk event.
func (s *Service) GetRiskEvent(ctx context.Context, merchantID, eventID string) (RiskEvent, error) {
	return s.repo.GetRiskEvent(ctx, merchantID, eventID)
}

// ListRiskEvents returns risk events for a merchant.
func (s *Service) ListRiskEvents(ctx context.Context, merchantID string, limit int, unresolvedOnly bool) ([]RiskEvent, error) {
	return s.repo.ListRiskEvents(ctx, merchantID, limit, unresolvedOnly)
}

// ResolveRiskEvent marks a risk event as manually reviewed and resolved.
func (s *Service) ResolveRiskEvent(ctx context.Context, merchantID, eventID, resolvedBy string) error {
	return s.repo.ResolveRiskEvent(ctx, merchantID, eventID, resolvedBy)
}
