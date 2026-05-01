package refund

import (
	"errors"
	"time"
)

// RefundState is the explicit state machine type for refunds.
type RefundState string

// RefundEvent drives transitions in the refund state machine.
type RefundEvent string

const (
	StateCreated    RefundState = "created"
	StateProcessing RefundState = "processing"
	StateProcessed  RefundState = "processed"
	StateFailed     RefundState = "failed"
)

const (
	EventInitiate RefundEvent = "initiate"   // created → processing
	EventSuccess  RefundEvent = "success"    // processing → processed
	EventFailure  RefundEvent = "failure"    // processing → failed
)

var (
	ErrInvalidTransition  = errors.New("invalid refund state transition")
	ErrRefundNotFound     = errors.New("refund not found")
	ErrPaymentNotCaptured = errors.New("refund requires a captured payment")
	ErrRefundAmountExceedsRefundable = errors.New("refund amount exceeds refundable balance")
	ErrZeroRefundAmount   = errors.New("refund amount must be greater than zero")
)

// Transition returns the next state given the current state and event,
// or ErrInvalidTransition if the event is not valid from the current state.
func Transition(from RefundState, ev RefundEvent) (RefundState, error) {
	table := map[RefundState]map[RefundEvent]RefundState{
		StateCreated: {
			EventInitiate: StateProcessing,
		},
		StateProcessing: {
			EventSuccess: StateProcessed,
			EventFailure: StateFailed,
		},
	}
	m, ok := table[from]
	if !ok {
		return "", ErrInvalidTransition
	}
	next, ok := m[ev]
	if !ok {
		return "", ErrInvalidTransition
	}
	return next, nil
}

// Refund is the core domain entity.
type Refund struct {
	ID              string
	PaymentID       string
	OrderID         string
	MerchantID      string
	Amount          int64
	Currency        string
	Reason          string
	Status          RefundState
	GatewayRefundID string
	Notes           map[string]any
	ProcessedAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// PaymentSnapshot carries the payment fields needed for eligibility checks.
type PaymentSnapshot struct {
	ID                    string
	OrderID               string
	MerchantID            string
	Amount                int64
	Currency              string
	Status                string
	AmountRefunded        int64
	AmountRefundedPending int64
}

// RefundableBalance returns how much of the payment can still be refunded,
// accounting for already-confirmed refunds and pending reservations.
func (p PaymentSnapshot) RefundableBalance() int64 {
	return p.Amount - p.AmountRefunded - p.AmountRefundedPending
}
