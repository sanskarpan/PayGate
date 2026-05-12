package dispute

import (
	"errors"
	"time"
)

// DisputeState is the explicit state machine type for disputes.
type DisputeState string

// DisputeEvent drives transitions in the dispute state machine.
type DisputeEvent string

const (
	StateOpen        DisputeState = "open"
	StateUnderReview DisputeState = "under_review"
	StateWon         DisputeState = "won"
	StateLost        DisputeState = "lost"
	StateAccepted    DisputeState = "accepted"
)

const (
	EventSubmitEvidence DisputeEvent = "submit_evidence" // attach evidence to an open dispute
	EventStartReview    DisputeEvent = "start_review"    // open → under_review
	EventWin            DisputeEvent = "win"             // open|under_review → won
	EventLose           DisputeEvent = "lose"            // open|under_review → lost
	EventAccept         DisputeEvent = "accept"          // open|under_review → accepted
)

var (
	ErrDisputeNotFound          = errors.New("dispute not found")
	ErrPaymentNotFound          = errors.New("payment not found")
	ErrSettlementNotFound       = errors.New("settlement not found")
	ErrInvalidTransition        = errors.New("invalid dispute state transition")
	ErrInvalidReason            = errors.New("invalid dispute reason")
	ErrInvalidAmount            = errors.New("invalid dispute amount")
	ErrInvalidCurrency          = errors.New("dispute currency does not match payment currency")
	ErrEvidenceAlreadySubmitted = errors.New("evidence already submitted for this dispute")
	ErrEvidenceNotAllowed       = errors.New("evidence can only be submitted while dispute is open")
	ErrDisputeTerminal          = errors.New("dispute is in a terminal state and cannot be transitioned")
)

// ValidReasons lists the accepted chargeback reason codes.
var ValidReasons = []string{
	"fraudulent",
	"product_not_received",
	"duplicate",
	"product_unacceptable",
	"credit_not_processed",
	"general",
}

// Transition returns the next DisputeState for the given event,
// or ErrInvalidTransition if the event is not valid from the current state.
func Transition(from DisputeState, ev DisputeEvent) (DisputeState, error) {
	if from == StateWon || from == StateLost || from == StateAccepted {
		return "", ErrDisputeTerminal
	}
	table := map[DisputeState]map[DisputeEvent]DisputeState{
		StateOpen: {
			EventStartReview: StateUnderReview,
			EventAccept:      StateAccepted,
			EventWin:         StateWon,
			EventLose:        StateLost,
		},
		StateUnderReview: {
			EventWin:    StateWon,
			EventLose:   StateLost,
			EventAccept: StateAccepted,
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

// Dispute represents a chargeback or payment dispute raised against a merchant.
type Dispute struct {
	ID                  string
	MerchantID          string
	PaymentID           string
	SettlementID        string
	Status              DisputeState
	Reason              string
	Amount              int64
	Currency            string
	Evidence            map[string]any
	EvidenceSubmittedAt *time.Time
	DueBy               *time.Time
	ResolvedAt          *time.Time
	Notes               string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// PaymentReference is the subset of payment data needed to validate dispute creation.
type PaymentReference struct {
	ID         string
	MerchantID string
	Amount     int64
	Currency   string
}

func isValidReason(reason string) bool {
	for _, valid := range ValidReasons {
		if reason == valid {
			return true
		}
	}
	return false
}
