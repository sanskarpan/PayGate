package dispute

import (
	"context"
	"fmt"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

// Service orchestrates dispute use-cases.
type Service struct {
	repo Repository
}

// NewService creates a new Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create opens a new dispute for the given merchant and payment.
func (s *Service) Create(ctx context.Context, merchantID, paymentID, settlementID, reason string, amount int64, currency string, dueBy *time.Time) (Dispute, error) {
	if paymentID == "" {
		return Dispute{}, ErrPaymentNotFound
	}
	if !isValidReason(reason) {
		return Dispute{}, ErrInvalidReason
	}
	if amount <= 0 {
		return Dispute{}, ErrInvalidAmount
	}

	paymentRef, err := s.repo.GetPaymentReference(ctx, merchantID, paymentID)
	if err != nil {
		return Dispute{}, err
	}
	if amount > paymentRef.Amount {
		return Dispute{}, ErrInvalidAmount
	}
	if currency == "" {
		currency = paymentRef.Currency
	}
	if currency != paymentRef.Currency {
		return Dispute{}, ErrInvalidCurrency
	}
	if settlementID != "" {
		exists, err := s.repo.SettlementExists(ctx, merchantID, settlementID)
		if err != nil {
			return Dispute{}, err
		}
		if !exists {
			return Dispute{}, ErrSettlementNotFound
		}
	}

	d := Dispute{
		ID:           idgen.New("disp"),
		MerchantID:   merchantID,
		PaymentID:    paymentID,
		SettlementID: settlementID,
		Status:       StateOpen,
		Reason:       reason,
		Amount:       amount,
		Currency:     currency,
		DueBy:        dueBy,
	}
	return s.repo.Create(ctx, d)
}

// Get returns a dispute by ID scoped to the merchant.
func (s *Service) Get(ctx context.Context, merchantID, id string) (Dispute, error) {
	return s.repo.GetByID(ctx, merchantID, id)
}

// List returns disputes for a merchant ordered by created_at DESC.
func (s *Service) List(ctx context.Context, merchantID string, limit int) ([]Dispute, error) {
	return s.repo.List(ctx, merchantID, limit)
}

// StartReview transitions a dispute from open to under_review.
func (s *Service) StartReview(ctx context.Context, merchantID, id string) (Dispute, error) {
	d, err := s.repo.GetByID(ctx, merchantID, id)
	if err != nil {
		return Dispute{}, err
	}
	if _, err := Transition(d.Status, EventStartReview); err != nil {
		return Dispute{}, err
	}
	return s.repo.UpdateStatus(ctx, merchantID, id, StateUnderReview, "")
}

// SubmitEvidence attaches evidence to a dispute.
// Returns ErrEvidenceAlreadySubmitted if evidence was previously submitted.
func (s *Service) SubmitEvidence(ctx context.Context, merchantID, id string, evidence map[string]any) (Dispute, error) {
	d, err := s.repo.GetByID(ctx, merchantID, id)
	if err != nil {
		return Dispute{}, err
	}
	if d.Status != StateOpen {
		return Dispute{}, ErrEvidenceNotAllowed
	}
	if d.EvidenceSubmittedAt != nil {
		return Dispute{}, ErrEvidenceAlreadySubmitted
	}
	return s.repo.SubmitEvidence(ctx, merchantID, id, evidence)
}

// Resolve closes a dispute with the given outcome (won, lost, or accepted).
func (s *Service) Resolve(ctx context.Context, merchantID, id string, outcome DisputeState, notes string) (Dispute, error) {
	if outcome != StateWon && outcome != StateLost && outcome != StateAccepted {
		return Dispute{}, fmt.Errorf("%w: outcome must be won, lost, or accepted", ErrInvalidTransition)
	}
	d, err := s.repo.GetByID(ctx, merchantID, id)
	if err != nil {
		return Dispute{}, err
	}
	var ev DisputeEvent
	switch outcome {
	case StateWon:
		ev = EventWin
	case StateLost:
		ev = EventLose
	case StateAccepted:
		ev = EventAccept
	}
	if _, err := Transition(d.Status, ev); err != nil {
		return Dispute{}, err
	}
	return s.repo.UpdateStatus(ctx, merchantID, id, outcome, notes)
}

// Accept marks a dispute as accepted (merchant concedes without contest).
func (s *Service) Accept(ctx context.Context, merchantID, id string) (Dispute, error) {
	d, err := s.repo.GetByID(ctx, merchantID, id)
	if err != nil {
		return Dispute{}, err
	}
	if _, err := Transition(d.Status, EventAccept); err != nil {
		return Dispute{}, err
	}
	return s.repo.UpdateStatus(ctx, merchantID, id, StateAccepted, "")
}
