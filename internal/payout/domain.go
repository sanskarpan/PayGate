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
	StateReturned   PayoutState = "returned"
	StateReversed   PayoutState = "reversed"
	StateCancelled  PayoutState = "cancelled"
)

const (
	EventInitiate PayoutEvent = "initiate" // pending → processing
	EventComplete PayoutEvent = "complete" // processing → completed
	EventFail     PayoutEvent = "fail"     // processing → failed
	EventReturn   PayoutEvent = "return"   // processing|completed -> returned
	EventReverse  PayoutEvent = "reverse"  // processing|completed -> reversed
	EventCancel   PayoutEvent = "cancel"   // pending|processing -> cancelled
)

var (
	ErrPayoutNotFound            = errors.New("payout not found")
	ErrInvalidTransition         = errors.New("invalid payout state transition")
	ErrPayoutAlreadyExists       = errors.New("payout already exists for this settlement")
	ErrSettlementNotProcessed    = errors.New("settlement must be processed before initiating payout")
	ErrSettlementOnHold          = errors.New("settlement is on hold")
	ErrInvalidRailSignature      = errors.New("invalid payout rail callback signature")
	ErrSimulatorScenarioNotFound = errors.New("payout simulator scenario not found")
	ErrInvalidSimulatorScenario  = errors.New("invalid payout simulator scenario")
)

// Transition returns the next PayoutState for the given event,
// or ErrInvalidTransition if the event is invalid from the current state.
func Transition(from PayoutState, ev PayoutEvent) (PayoutState, error) {
	table := map[PayoutState]map[PayoutEvent]PayoutState{
		StatePending: {
			EventInitiate: StateProcessing,
			EventCancel:   StateCancelled,
		},
		StateProcessing: {
			EventComplete: StateCompleted,
			EventFail:     StateFailed,
			EventReturn:   StateReturned,
			EventReverse:  StateReversed,
			EventCancel:   StateCancelled,
		},
		StateCompleted: {
			EventReturn:  StateReturned,
			EventReverse: StateReversed,
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
	ID             string
	MerchantID     string
	SettlementID   string
	BeneficiaryID  string
	SagaID         string
	Status         PayoutState
	ApprovalStatus ApprovalStatus
	Amount         int64
	Currency       string
	BankReference  string
	RailReference  string
	FailureReason  string
	ReturnReason   string
	InitiatedAt    *time.Time
	CompletedAt    *time.Time
	FailedAt       *time.Time
	ReturnedAt     *time.Time
	ReversedAt     *time.Time
	CancelledAt    *time.Time
	CancelReason   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type TimelineEvent struct {
	ID              string
	PayoutID        string
	MerchantID      string
	EventType       string
	StatusBefore    PayoutState
	StatusAfter     PayoutState
	CallbackEventID string
	Payload         map[string]any
	CreatedAt       time.Time
}

type RailCallbackStatus string

const (
	RailStatusProcessing RailCallbackStatus = "processing"
	RailStatusCompleted  RailCallbackStatus = "completed"
	RailStatusFailed     RailCallbackStatus = "failed"
	RailStatusReturned   RailCallbackStatus = "returned"
	RailStatusReversed   RailCallbackStatus = "reversed"
)

type RailCallback struct {
	EventID       string
	PayoutID      string
	MerchantID    string
	Status        RailCallbackStatus
	RailReference string
	Reason        string
	OccurredAt    time.Time
}

type SimulatorScenarioStep struct {
	Status            RailCallbackStatus `json:"status"`
	DelayMilliseconds int                `json:"delay_ms"`
	RailReference     string             `json:"rail_reference,omitempty"`
	Reason            string             `json:"reason,omitempty"`
	DuplicateCount    int                `json:"duplicate_count,omitempty"`
}

type SimulatorScenario struct {
	ID                         string                  `json:"id,omitempty"`
	MerchantID                 string                  `json:"merchant_id,omitempty"`
	SettlementID               string                  `json:"settlement_id,omitempty"`
	TransientFailuresRemaining int                     `json:"transient_failures_remaining"`
	Steps                      []SimulatorScenarioStep `json:"steps"`
	Notes                      string                  `json:"notes,omitempty"`
	CreatedAt                  time.Time               `json:"created_at,omitempty"`
	UpdatedAt                  time.Time               `json:"updated_at,omitempty"`
}
