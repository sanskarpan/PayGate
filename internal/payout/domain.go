package payout

import (
	"errors"
	"time"
)

// PayoutState is the explicit state machine type for payouts.
type PayoutState string

// PayoutEvent drives transitions in the payout state machine.
type PayoutEvent string

const (
	StatePending    PayoutState = "pending"
	StateProcessing PayoutState = "processing"
	StateCompleted  PayoutState = "completed"
	StateFailed     PayoutState = "failed"
)

const (
	EventInitiate PayoutEvent = "initiate" // pending → processing
	EventComplete PayoutEvent = "complete" // processing → completed
	EventFail     PayoutEvent = "fail"     // processing → failed
)

var (
	ErrPayoutNotFound         = errors.New("payout not found")
	ErrInvalidTransition      = errors.New("invalid payout state transition")
	ErrPayoutAlreadyExists    = errors.New("payout already exists for this settlement")
	ErrSettlementNotProcessed = errors.New("settlement must be processed before initiating payout")
	ErrSettlementOnHold       = errors.New("settlement is on hold")
)

// Transition returns the next PayoutState for the given event,
// or ErrInvalidTransition if the event is invalid from the current state.
func Transition(from PayoutState, ev PayoutEvent) (PayoutState, error) {
	table := map[PayoutState]map[PayoutEvent]PayoutState{
		StatePending: {
			EventInitiate: StateProcessing,
		},
		StateProcessing: {
			EventComplete: StateCompleted,
			EventFail:     StateFailed,
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

// Payout represents a bank transfer of a settlement's net amount to a merchant.
type Payout struct {
	ID            string
	MerchantID    string
	SettlementID  string
	Status        PayoutState
	Amount        int64
	Currency      string
	BankReference string
	FailureReason string
	InitiatedAt   *time.Time
	CompletedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
