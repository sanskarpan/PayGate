package risk

import (
	"context"
	"log/slog"
	"time"

	"github.com/sanskarpan/PayGate/internal/payment"
)

// Service evaluates payment risk and manages risk event records.
type Service struct {
	repo             Repository
	logger           *slog.Logger
	alertFn          AlertFunc
	payments         *payment.Service
	reserveEscalator func(ctx context.Context, ev RiskEvent) error

	asyncSem     chan struct{}
	asyncTimeout time.Duration
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
	svc := &Service{
		repo:         repo,
		logger:       logger,
		asyncSem:     make(chan struct{}, 32),
		asyncTimeout: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func WithPaymentService(payments *payment.Service) func(*Service) {
	return func(s *Service) {
		s.payments = payments
	}
}

func WithReserveEscalator(fn func(ctx context.Context, ev RiskEvent) error) func(*Service) {
	return func(s *Service) {
		s.reserveEscalator = fn
	}
}

func WithAsyncConfig(limit int, timeout time.Duration) func(*Service) {
	return func(s *Service) {
		if limit > 0 {
			s.asyncSem = make(chan struct{}, limit)
		}
		if timeout > 0 {
			s.asyncTimeout = timeout
		}
	}
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
	cfg, err := s.repo.GetMerchantFraudConfig(ctx, in.MerchantID)
	if err != nil {
		s.logger.Warn("fetch merchant fraud config failed", "error", err)
		cfg = DefaultMerchantFraudConfig(in.MerchantID)
	}

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

	deviceHourly := 0
	if in.DeviceFingerprint != "" {
		deviceHourly, err = s.repo.UpsertVelocityCounter(ctx, "device_fingerprint", in.DeviceFingerprint, VelocityWindow1H, in.Amount)
		if err != nil {
			s.logger.Warn("upsert device velocity counter failed", "error", err)
			deviceHourly = 0
		}
	}

	if in.MerchantAvgTxn == 0 {
		avg, err := s.repo.MerchantAverageTxnAmount(ctx, in.MerchantID)
		if err != nil {
			s.logger.Warn("fetch merchant avg txn failed", "error", err)
		}
		in.MerchantAvgTxn = avg
	}

	result := Evaluate(in, cfg, merchantHourly, ipHourly, deviceHourly)

	reviewStatus := ReviewStatusNotRequired
	if result.Action == RiskActionHold {
		reviewStatus = ReviewStatusPending
	}

	ev, err := s.repo.CreateRiskEvent(ctx, RiskEvent{
		MerchantID:            in.MerchantID,
		PaymentID:             in.PaymentID,
		Score:                 result.Score,
		Action:                result.Action,
		TriggeredRules:        result.TriggeredRules,
		DeviceFingerprintHash: in.DeviceFingerprint,
		BrowserLanguage:       in.BrowserLanguage,
		UserAgent:             in.UserAgent,
		CardBIN:               in.CardBIN,
		CardNetwork:           in.CardNetwork,
		IssuerCountry:         in.IssuerCountry,
		CardCountry:           in.CardCountry,
		FundingType:           in.FundingType,
		ReviewStatus:          reviewStatus,
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
			s.runAsync(context.WithoutCancel(ctx), "risk alert", ev, func(asyncCtx context.Context) error {
				s.alertFn(asyncCtx, ev)
				return nil
			})
		}
		if s.reserveEscalator != nil && shouldEscalateReserve(ev) {
			s.runAsync(context.WithoutCancel(ctx), "reserve escalation", ev, func(asyncCtx context.Context) error {
				return s.reserveEscalator(asyncCtx, ev)
			})
		}
	}

	return ev, nil
}

func (s *Service) runAsync(ctx context.Context, operation string, ev RiskEvent, fn func(context.Context) error) {
	select {
	case s.asyncSem <- struct{}{}:
	default:
		s.logger.Warn("risk async callback dropped because queue is full",
			"operation", operation,
			"merchant_id", ev.MerchantID,
			"payment_id", ev.PaymentID,
		)
		return
	}

	go func() {
		defer func() { <-s.asyncSem }()
		timeoutCtx, cancel := context.WithTimeout(ctx, s.asyncTimeout)
		defer cancel()
		if err := fn(timeoutCtx); err != nil {
			s.logger.Warn(operation+" failed", "merchant_id", ev.MerchantID, "payment_id", ev.PaymentID, "error", err)
		}
	}()
}

func shouldEscalateReserve(ev RiskEvent) bool {
	if ev.Score >= 90 {
		return true
	}
	for _, rule := range ev.TriggeredRules {
		switch rule {
		case "blocked_country", "blocked_bin", "merchant_velocity_1h":
			return true
		}
	}
	return false
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

func (s *Service) AssignRiskEvent(ctx context.Context, merchantID, eventID, assignedTo string) error {
	return s.repo.AssignRiskEvent(ctx, merchantID, eventID, assignedTo)
}

func (s *Service) ReviewRiskEvent(ctx context.Context, merchantID, eventID, decision, notes, actor string) (RiskEvent, error) {
	ev, err := s.repo.GetRiskEvent(ctx, merchantID, eventID)
	if err != nil {
		return RiskEvent{}, err
	}
	status := ReviewStatusBlocked
	if decision == "approve" {
		status = ReviewStatusApproved
		if s.payments != nil {
			paymentRecord, err := s.payments.Get(ctx, merchantID, ev.PaymentID)
			if err != nil {
				return RiskEvent{}, err
			}
			if paymentRecord.Status == payment.StateAuthorized {
				if _, err := s.payments.CaptureForMerchant(ctx, merchantID, ev.PaymentID, paymentRecord.Amount); err != nil {
					return RiskEvent{}, err
				}
			}
		}
	} else if s.payments != nil {
		if _, err := s.payments.ReverseAuthorization(ctx, merchantID, ev.PaymentID, "risk review blocked"); err != nil {
			return RiskEvent{}, err
		}
	}
	return s.repo.ReviewRiskEvent(ctx, merchantID, eventID, status, notes, actor)
}

func (s *Service) GetMerchantFraudConfig(ctx context.Context, merchantID string) (MerchantFraudConfig, error) {
	return s.repo.GetMerchantFraudConfig(ctx, merchantID)
}

func (s *Service) UpsertMerchantFraudConfig(ctx context.Context, cfg MerchantFraudConfig) (MerchantFraudConfig, error) {
	return s.repo.UpsertMerchantFraudConfig(ctx, cfg)
}
