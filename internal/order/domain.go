package order

import (
	"errors"
	"strings"
	"time"
)

type State string

type Event string

const (
	StateCreated   State = "created"
	StateAttempted State = "attempted"
	StatePaid      State = "paid"
	StateFailed    State = "failed"
	StateExpired   State = "expired"
)

const (
	EventAttempted Event = "attempted"
	EventPaid      Event = "paid"
	EventFailed    Event = "failed"
	EventExpired   Event = "expired"
)

var (
	ErrInvalidAmount     = errors.New("amount must be positive")
	ErrInvalidCurrency   = errors.New("currency must be INR or USD")
	ErrInvalidTransition = errors.New("invalid state transition")
)

type Order struct {
	ID             string
	MerchantID     string
	IdempotencyKey string
	Amount         int64
	AmountPaid     int64
	AmountDue      int64
	Currency       string
	Receipt        string
	Status         State
	PartialPayment bool
	Notes          map[string]any
	ExpiresAt      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (o Order) ValidateForCreate() error {
	if o.Amount <= 0 {
		return ErrInvalidAmount
	}
	if o.Currency != "INR" && o.Currency != "USD" {
		return ErrInvalidCurrency
	}
	if strings.TrimSpace(o.MerchantID) == "" {
		return errors.New("merchant id is required")
	}
	return nil
}

func Transition(from State, event Event) (State, error) {
	table := map[State]map[Event]State{
		StateCreated: {
			EventAttempted: StateAttempted,
			EventExpired:   StateExpired,
		},
		StateAttempted: {
			EventPaid:   StatePaid,
			EventFailed: StateFailed,
		},
	}

	nextStates, ok := table[from]
	if !ok {
		return "", ErrInvalidTransition
	}
	next, ok := nextStates[event]
	if !ok {
		return "", ErrInvalidTransition
	}
	return next, nil
}
