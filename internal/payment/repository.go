package payment

import (
	"context"
	"time"
)

type CaptureResult struct {
	PaymentID              string
	MerchantID             string
	OrderID                string
	Amount                 int64
	Currency               string
	Method                 string
	Provider               string
	RoutingReason          string
	AttemptedProviders     []string
	PaymentMethodTokenID   string
	CardTokenClass         string
	CardBrand              string
	CardLast4              string
	CardExpMonth           int
	CardExpYear            int
	CardIssuerName         string
	CardIssuerCountry      string
	CardCountry            string
	FundingType            string
	NetworkTokenType       string
	MethodState            string
	MethodStateReason      string
	Status                 PaymentState
	ChallengeSessionID     string
	ChallengeStatus        string
	ChallengeRedirectURL   string
	ChallengeCallbackToken string
	ChallengeExpiresAt     *time.Time
	Captured               bool
	CapturedAt             *time.Time
	CreatedAt              time.Time
	AuthorizedAt           *time.Time
	Splits                 []PaymentSplit
}

type Repository interface {
	StartAuthorization(ctx context.Context, in CreateAuthorizedInput) (CaptureResult, error)
	AttachCardPaymentDetails(ctx context.Context, merchantID, paymentID string, in CardPaymentDetailsInput) error
	UpdateGatewayRouting(ctx context.Context, merchantID, paymentID string, decision GatewayRouteDecision) error
	CreateFailedAttempt(ctx context.Context, in CreateAuthorizedInput, errorCode, errorDescription string) error
	MarkAuthorizationAuthorized(ctx context.Context, merchantID, paymentID, gatewayReference, authCode string, autoCaptureAt *time.Time) (CaptureResult, error)
	MarkAuthorizationRequiresAction(ctx context.Context, merchantID, paymentID string, in CardChallengeSessionInput) (CaptureResult, error)
	MarkAuthorizationFailed(ctx context.Context, merchantID, paymentID, errorCode, errorDescription string) error
	CompleteCardChallenge(ctx context.Context, merchantID, paymentID, callbackToken, gatewayReference, authCode string, completedAt time.Time) (CaptureResult, bool, error)
	FailCardChallenge(ctx context.Context, merchantID, paymentID, callbackToken, errorCode, errorDescription string, status string, failedAt time.Time) (CaptureResult, bool, error)
	// ResolveCardChallengeMerchant returns the merchant that owns the challenge
	// session identified by paymentID and callbackToken. Gateways calling the
	// public callback URL only ever hold those two values.
	ResolveCardChallengeMerchant(ctx context.Context, paymentID, callbackToken string) (string, error)
	// ResolveUPIIntentMerchant is the equivalent lookup for UPI intent callbacks.
	ResolveUPIIntentMerchant(ctx context.Context, paymentID, callbackToken string) (string, error)
	CreateUPIIntent(ctx context.Context, in CreateUPIIntentRecordInput) (UPIIntentResult, error)
	AttachUPIIntentGatewayData(ctx context.Context, merchantID, paymentID string, gatewayResult GatewayUPIIntentResult) (UPIIntentResult, error)
	GetUPIIntent(ctx context.Context, merchantID, paymentID string) (UPIIntentResult, error)
	PollUPIIntent(ctx context.Context, merchantID, paymentID string, polledAt time.Time) error
	MarkUPIIntentProcessing(ctx context.Context, paymentID string, eventID string) (UPIIntentResult, bool, error)
	CompleteUPIIntent(ctx context.Context, paymentID string, eventID string, gatewayReference string, processedAt time.Time) (UPIIntentResult, bool, error)
	FailUPIIntent(ctx context.Context, paymentID string, eventID string, providerStatus UPIProviderStatus, errorCode, errorDescription string, processedAt time.Time) (UPIIntentResult, bool, error)
	AbandonUPIIntent(ctx context.Context, merchantID, paymentID, reason string, abandonedAt time.Time) (UPIIntentResult, error)
	ExpireUPIIntent(ctx context.Context, merchantID, paymentID string, expiredAt time.Time) (UPIIntentResult, error)
	CreateRedirectSession(ctx context.Context, in CreateRedirectSessionRecordInput) (RedirectSessionResult, error)
	AttachRedirectGatewayData(ctx context.Context, merchantID, paymentID string, gatewayResult GatewayRedirectResult) (RedirectSessionResult, error)
	GetRedirectSession(ctx context.Context, merchantID, paymentID string) (RedirectSessionResult, error)
	PollRedirectSession(ctx context.Context, merchantID, paymentID string, polledAt time.Time) error
	MarkRedirectProcessing(ctx context.Context, paymentID string, eventID string) (RedirectSessionResult, bool, error)
	CompleteRedirectSession(ctx context.Context, paymentID string, eventID string, gatewayReference string, processedAt time.Time) (RedirectSessionResult, bool, error)
	FailRedirectSession(ctx context.Context, paymentID string, eventID string, providerStatus RedirectProviderStatus, errorCode, errorDescription string, processedAt time.Time) (RedirectSessionResult, bool, error)
	AbandonRedirectSession(ctx context.Context, merchantID, paymentID, reason string, abandonedAt time.Time) (RedirectSessionResult, error)
	ExpireRedirectSession(ctx context.Context, merchantID, paymentID string, expiredAt time.Time) (RedirectSessionResult, error)
	ReverseAuthorization(ctx context.Context, merchantID, paymentID, reason string) (CaptureResult, error)
	CaptureAuthorizedPayment(ctx context.Context, merchantID, paymentID string, amount int64) (CaptureResult, error)
	GetPayment(ctx context.Context, merchantID, paymentID string) (CaptureResult, error)
	ListPayments(ctx context.Context, f ListFilter) (ListResult, error)
	AutoCaptureDue(ctx context.Context) (int64, error)
	ExpireAuthorizationWindow(ctx context.Context, window time.Duration) (int64, error)
}

type ListFilter struct {
	MerchantID string
	OrderID    string
	Count      int
}

type ListResult struct {
	Items []CaptureResult
}

type CreateAuthorizedInput struct {
	PaymentID            string
	MerchantID           string
	OrderID              string
	Amount               int64
	Currency             string
	Method               string
	Provider             string
	RoutingReason        string
	AttemptedProviders   []string
	PaymentMethodTokenID string
	CardTokenClass       string
	CardBrand            string
	CardLast4            string
	CardExpMonth         int
	CardExpYear          int
	MethodState          string
	MethodStateReason    string
	IdempotencyKey       string
	GatewayReference     string
	AuthCode             string
	AutoCaptureAt        *time.Time
	Splits               []CreatePaymentSplitInput
}

type CardPaymentDetailsInput struct {
	PaymentMethodTokenID string
	CardTokenClass       string
	CardBrand            string
	CardLast4            string
	CardExpMonth         int
	CardExpYear          int
	CardIssuerName       string
	CardIssuerCountry    string
	CardCountry          string
	FundingType          string
	NetworkTokenType     string
}

type CardChallengeSessionInput struct {
	SessionID            string
	CardTokenID          string
	RedirectURL          string
	ChallengeReference   string
	CallbackToken        string
	AutoCaptureRequested bool
	ExpiresAt            time.Time
}

type CreateUPIIntentRecordInput struct {
	PaymentID      string
	MerchantID     string
	OrderID        string
	Amount         int64
	Currency       string
	Method         string
	IdempotencyKey string
	FlowType       string
	MandateID      string
	QRMode         string
	QRPayload      string
	QRImageURL     string
	DisplayName    string
	IsReusable     bool
	VPA            string
	ExpiresAt      time.Time
	CallbackToken  string
}

type CreatePaymentSplitInput struct {
	DestinationType  string
	DestinationRef   string
	BeneficiaryLabel string
	Amount           int64
	Currency         string
}

type PaymentSplit struct {
	ID               string
	MerchantID       string
	PaymentID        string
	DestinationType  string
	DestinationRef   string
	BeneficiaryLabel string
	Amount           int64
	Currency         string
	CreatedAt        time.Time
}

type UPIIntentResult struct {
	CaptureResult
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
	CompletedAt        *time.Time
	LastPolledAt       *time.Time
	FailureCode        string
	FailureDescription string
}

type RedirectProviderStatus string

const (
	RedirectProviderStatusPending   RedirectProviderStatus = "pending"
	RedirectProviderStatusSucceeded RedirectProviderStatus = "succeeded"
	RedirectProviderStatusFailed    RedirectProviderStatus = "failed"
	RedirectProviderStatusExpired   RedirectProviderStatus = "expired"
	RedirectProviderStatusAbandoned RedirectProviderStatus = "abandoned"
)

type RedirectSessionResult struct {
	CaptureResult
	FlowType           string
	BankCode           string
	BankName           string
	WalletCode         string
	WalletName         string
	RedirectURL        string
	GatewayReference   string
	ProviderStatus     RedirectProviderStatus
	CallbackToken      string
	ExpiresAt          time.Time
	CompletedAt        *time.Time
	LastPolledAt       *time.Time
	FailureCode        string
	FailureDescription string
}

type CreateRedirectSessionRecordInput struct {
	PaymentID      string
	MerchantID     string
	OrderID        string
	Amount         int64
	Currency       string
	Method         string
	IdempotencyKey string
	FlowType       string
	BankCode       string
	BankName       string
	WalletCode     string
	WalletName     string
	ExpiresAt      time.Time
	CallbackToken  string
	Provider       string
	RoutingReason  string
	Attempted      []string
}
