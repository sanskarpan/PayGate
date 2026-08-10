package order

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
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
	ErrInvalidCursor     = errors.New("cursor is not a valid pagination cursor")
	ErrReceiptTooLong    = errors.New("receipt must be at most 40 characters")
	ErrTooManyNotes      = errors.New("notes must contain at most 15 keys")
	ErrNoteTooLong       = errors.New("each notes key and value must be at most 256 characters")
)

// Limits published in docs/API-CONTRACTS.md. They were documented but never
// enforced, so a 100,000-character receipt was accepted and stored in full.
const (
	MaxReceiptLength = 40
	MaxNoteKeys      = 15
	MaxNoteLength    = 256
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
	if utf8.RuneCountInString(o.Receipt) > MaxReceiptLength {
		return ErrReceiptTooLong
	}
	if err := validateNotes(o.Notes); err != nil {
		return err
	}
	return nil
}

// validateNotes enforces the documented notes limits. Length is counted in
// runes so a multi-byte value is not penalised for its encoding.
func validateNotes(notes map[string]any) error {
	if len(notes) > MaxNoteKeys {
		return ErrTooManyNotes
	}
	for key, value := range notes {
		if utf8.RuneCountInString(key) > MaxNoteLength {
			return ErrNoteTooLong
		}
		if s, ok := value.(string); ok && utf8.RuneCountInString(s) > MaxNoteLength {
			return ErrNoteTooLong
		}
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
