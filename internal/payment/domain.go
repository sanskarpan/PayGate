package payment

import (
	"errors"
	"time"
)

type PaymentState string

type PaymentEvent string

const (
	StateCreated               PaymentState = "created"
	StatePendingCustomerAction PaymentState = "pending_customer_action"
	StateProcessing            PaymentState = "processing"
	StateAuthorized            PaymentState = "authorized"
	StateAuthorizationReversed PaymentState = "authorization_reversed"
	StateCaptured              PaymentState = "captured"
	StateFailed                PaymentState = "failed"
	StateAutoRefunded          PaymentState = "auto_refunded"
)

const (
	EventCustomerActionRequired PaymentEvent = "customer_action_required"
	EventProcessingStarted      PaymentEvent = "processing_started"
	EventAuthSuccess            PaymentEvent = "auth_success"
	EventAuthFailed             PaymentEvent = "auth_failed"
	EventAuthReversed           PaymentEvent = "auth_reversed"
	EventCapture                PaymentEvent = "capture"
	EventCaptureExpiry          PaymentEvent = "capture_expiry"
)

var (
	ErrInvalidTransition     = errors.New("invalid payment transition")
	ErrOrderNotFound         = errors.New("order not found")
	ErrOrderExpired          = errors.New("order is expired")
	ErrCurrencyMismatch      = errors.New("payment currency must match order currency")
	ErrAmountMismatch        = errors.New("payment amount does not match order constraints")
	ErrAuthorizationDeclined = errors.New("payment authorization declined by gateway")
	ErrInvalidPaymentAmount  = errors.New("payment amount must be greater than zero")
	ErrUPIIntentNotFound     = errors.New("upi intent not found")
	ErrUPICallbackRejected   = errors.New("upi callback rejected")
)

func Transition(from PaymentState, ev PaymentEvent) (PaymentState, error) {
	table := map[PaymentState]map[PaymentEvent]PaymentState{
		StateCreated: {
			EventAuthSuccess:            StateAuthorized,
			EventAuthFailed:             StateFailed,
			EventCustomerActionRequired: StatePendingCustomerAction,
		},
		StatePendingCustomerAction: {
			EventProcessingStarted: StateProcessing,
			EventCapture:           StateCaptured,
			EventAuthFailed:        StateFailed,
		},
		StateProcessing: {
			EventCapture:    StateCaptured,
			EventAuthFailed: StateFailed,
		},
		StateAuthorized: {
			EventAuthReversed:  StateAuthorizationReversed,
			EventCapture:       StateCaptured,
			EventCaptureExpiry: StateAutoRefunded,
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

type Attempt struct {
	ID               string
	OrderID          string
	MerchantID       string
	PaymentID        string
	Amount           int64
	Currency         string
	Method           string
	Status           string
	GatewayReference string
	ErrorCode        string
	ErrorDescription string
}

type Payment struct {
	ID                string
	AttemptID         string
	OrderID           string
	MerchantID        string
	Amount            int64
	Currency          string
	Method            string
	Status            PaymentState
	MethodState       string
	MethodStateReason string
	Captured          bool
	GatewayReference  string
	AuthCode          string
}

const (
	MethodStateCardAuthorizationStarted  = "card_authorization_started"
	MethodStateCardAuthorized            = "card_authorized"
	MethodStateCardDeclined              = "card_declined"
	MethodStateCardAuthorizationReversed = "card_authorization_reversed"
	MethodStateCardCaptured              = "card_captured"
	MethodStateUPIIntentCreated          = "upi_intent_created"
	MethodStateUPIPendingCustomerAction  = "upi_pending_customer_action"
	MethodStateUPIProcessing             = "upi_processing"
	MethodStateUPICollected              = "upi_collected"
	MethodStateUPIFailed                 = "upi_failed"
	MethodStateUPIExpired                = "upi_expired"
	MethodStateUPIAbandoned              = "upi_abandoned"
)

type UPIProviderStatus string

const (
	UPIProviderStatusPending   UPIProviderStatus = "pending"
	UPIProviderStatusSucceeded UPIProviderStatus = "succeeded"
	UPIProviderStatusFailed    UPIProviderStatus = "failed"
	UPIProviderStatusExpired   UPIProviderStatus = "expired"
	UPIProviderStatusAbandoned UPIProviderStatus = "abandoned"
)

type UPIIntent struct {
	PaymentID          string
	MerchantID         string
	OrderID            string
	VPA                string
	DeepLink           string
	IntentURI          string
	GatewayReference   string
	ProviderStatus     UPIProviderStatus
	CallbackToken      string
	ExpiresAt          time.Time
	LastPolledAt       *time.Time
	CompletedAt        *time.Time
	FailureCode        string
	FailureDescription string
}
