package payment

import (
	"errors"
	"time"
)

type PaymentState string

type PaymentEvent string

const (
	StateCreated               PaymentState = "created"
	StateRequiresAction        PaymentState = "requires_action"
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
	EventCustomerActionComplete PaymentEvent = "customer_action_complete"
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
	ErrInvalidUPIVPA         = errors.New("invalid upi vpa")
	ErrUPICallbackRejected   = errors.New("upi callback rejected")
	ErrCardChallengeRejected = errors.New("card challenge callback rejected")
)

func Transition(from PaymentState, ev PaymentEvent) (PaymentState, error) {
	table := map[PaymentState]map[PaymentEvent]PaymentState{
		StateCreated: {
			EventAuthSuccess:            StateAuthorized,
			EventAuthFailed:             StateFailed,
			EventCustomerActionRequired: StateRequiresAction,
		},
		StateRequiresAction: {
			EventCustomerActionComplete: StateAuthorized,
			EventAuthFailed:             StateFailed,
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
	MethodStateCard3DSRequired           = "card_3ds_required"
	MethodStateCard3DSAuthenticated      = "card_3ds_authenticated"
	MethodStateCard3DSFailed             = "card_3ds_failed"
	MethodStateCard3DSExpired            = "card_3ds_expired"
	MethodStateCard3DSAbandoned          = "card_3ds_abandoned"
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
	MethodStateNetbankingRedirectCreated = "netbanking_redirect_created"
	MethodStateNetbankingProcessing      = "netbanking_processing"
	MethodStateNetbankingSucceeded       = "netbanking_succeeded"
	MethodStateNetbankingFailed          = "netbanking_failed"
	MethodStateNetbankingExpired         = "netbanking_expired"
	MethodStateNetbankingAbandoned       = "netbanking_abandoned"
	MethodStateWalletRedirectCreated     = "wallet_redirect_created"
	MethodStateWalletProcessing          = "wallet_processing"
	MethodStateWalletSucceeded           = "wallet_succeeded"
	MethodStateWalletFailed              = "wallet_failed"
	MethodStateWalletExpired             = "wallet_expired"
	MethodStateWalletAbandoned           = "wallet_abandoned"
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
	FlowType           string
	MandateID          string
	QRMode             string
	QRPayload          string
	QRImageURL         string
	DisplayName        string
	IsReusable         bool
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
