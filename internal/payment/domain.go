package payment

import "errors"

type PaymentState string

type PaymentEvent string

const (
	StateCreated      PaymentState = "created"
	StateAuthorized   PaymentState = "authorized"
	StateCaptured     PaymentState = "captured"
	StateFailed       PaymentState = "failed"
	StateAutoRefunded PaymentState = "auto_refunded"
)

const (
	EventAuthSuccess   PaymentEvent = "auth_success"
	EventAuthFailed    PaymentEvent = "auth_failed"
	EventCapture       PaymentEvent = "capture"
	EventCaptureExpiry PaymentEvent = "capture_expiry"
)

var (
	ErrInvalidTransition = errors.New("invalid payment transition")
	ErrOrderNotFound     = errors.New("order not found")
	ErrOrderExpired      = errors.New("order is expired")
	ErrCurrencyMismatch  = errors.New("payment currency must match order currency")
	ErrAmountMismatch    = errors.New("payment amount does not match order constraints")
)

func Transition(from PaymentState, ev PaymentEvent) (PaymentState, error) {
	table := map[PaymentState]map[PaymentEvent]PaymentState{
		StateCreated: {
			EventAuthSuccess: StateAuthorized,
			EventAuthFailed:  StateFailed,
		},
		StateAuthorized: {
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
	ID               string
	AttemptID        string
	OrderID          string
	MerchantID       string
	Amount           int64
	Currency         string
	Method           string
	Status           PaymentState
	Captured         bool
	GatewayReference string
	AuthCode         string
}
