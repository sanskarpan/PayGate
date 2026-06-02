package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/common/metrics"
	"github.com/sanskarpan/PayGate/internal/tokenization"
	"github.com/sanskarpan/PayGate/internal/upiverify"
)

var ErrPaymentNotFound = errors.New("payment not found")
var ErrPaymentMethodTokenRequired = errors.New("payment_method_token_id is required for card payments")
var ErrPaymentMethodTokenInvalid = errors.New("payment method token is invalid or unavailable")

type GatewayClient interface {
	Authorize(ctx context.Context, amount int64, currency, merchantID, method string) (GatewayAuthResult, error)
}

type NamedGateway interface {
	Name() string
}

type RoutedGateway interface {
	AuthorizeWithRouting(ctx context.Context, amount int64, currency, merchantID, method string) (GatewayAuthResult, GatewayRouteDecision, error)
}

type UPIGateway interface {
	CreateUPIIntent(ctx context.Context, in GatewayUPIIntentCreateInput) (GatewayUPIIntentResult, error)
	PollUPIIntent(ctx context.Context, in GatewayUPIIntentPollInput) (GatewayUPIIntentStatus, error)
}

type RedirectGateway interface {
	CreateRedirectSession(ctx context.Context, in GatewayRedirectCreateInput) (GatewayRedirectResult, error)
	PollRedirectSession(ctx context.Context, in GatewayRedirectPollInput) (GatewayRedirectStatus, error)
}

type CardTokenAuthorizer interface {
	PrepareAuthorization(ctx context.Context, merchantID, tokenID, paymentID string) (tokenization.CardTokenReference, error)
	CompleteAuthorization(ctx context.Context, merchantID, tokenID, paymentID string, success bool, reason string) error
}

type GatewayAuthResult struct {
	Success          bool
	RequiresAction   bool
	GatewayReference string
	AuthCode         string
	ChallengeURL     string
	ChallengeToken   string
	ChallengeExpiry  *time.Time
	ErrorCode        string
	ErrorDescription string
}

type GatewayRouteDecision struct {
	Provider           string
	RoutingReason      string
	AttemptedProviders []string
	PrimaryProvider    string
	FallbackUsed       bool
}

type GatewayUPIIntentCreateInput struct {
	PaymentID   string
	MerchantID  string
	OrderID     string
	Amount      int64
	Currency    string
	FlowType    string
	MandateID   string
	QRMode      string
	QRPayload   string
	QRImageURL  string
	IsReusable  bool
	DisplayName string
	VPA         string
	ExpiresAt   time.Time
}

type GatewayUPIIntentResult struct {
	GatewayReference string
	DeepLink         string
	IntentURI        string
	QRPayload        string
	QRImageURL       string
	ExpiresAt        time.Time
}

type GatewayUPIIntentPollInput struct {
	PaymentID        string
	MerchantID       string
	OrderID          string
	GatewayReference string
	CreatedAt        time.Time
	ExpiresAt        time.Time
}

type GatewayUPIIntentStatus struct {
	ProviderStatus   UPIProviderStatus
	GatewayReference string
	ErrorCode        string
	ErrorDescription string
}

type GatewayRedirectCreateInput struct {
	PaymentID  string
	MerchantID string
	OrderID    string
	Amount     int64
	Currency   string
	Method     string
	BankCode   string
	WalletCode string
	ExpiresAt  time.Time
}

type GatewayRedirectResult struct {
	GatewayReference string
	RedirectURL      string
	BankCode         string
	BankName         string
	WalletCode       string
	WalletName       string
	ExpiresAt        time.Time
}

type GatewayRedirectPollInput struct {
	PaymentID        string
	MerchantID       string
	OrderID          string
	Method           string
	GatewayReference string
	CreatedAt        time.Time
	ExpiresAt        time.Time
}

type GatewayRedirectStatus struct {
	ProviderStatus   RedirectProviderStatus
	GatewayReference string
	ErrorCode        string
	ErrorDescription string
}

type Service struct {
	repo       Repository
	gateway    GatewayClient
	upi        UPIGateway
	redirect   RedirectGateway
	cardTokens CardTokenAuthorizer
}

func NewService(repo Repository, gw GatewayClient, opts ...func(*Service)) *Service {
	svc := &Service{repo: repo, gateway: gw}
	if upi, ok := gw.(UPIGateway); ok {
		svc.upi = upi
	}
	if redirect, ok := gw.(RedirectGateway); ok {
		svc.redirect = redirect
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func WithCardTokenAuthorizer(authorizer CardTokenAuthorizer) func(*Service) {
	return func(s *Service) {
		s.cardTokens = authorizer
	}
}

type AuthorizeInput struct {
	PaymentID            string                      `json:"-"`
	MerchantID           string                      `json:"-"`
	OrderID              string                      `json:"order_id"`
	Amount               int64                       `json:"amount"`
	Currency             string                      `json:"currency"`
	Method               string                      `json:"method"`
	PaymentMethodTokenID string                      `json:"payment_method_token_id"`
	IdempotencyKey       string                      `json:"-"`
	AutoCapture          bool                        `json:"auto_capture"`
	RiskContext          *AuthorizeRiskContext       `json:"risk_context,omitempty"`
	SplitInstructions    []AuthorizeSplitInstruction `json:"split_instructions,omitempty"`
}

type AuthorizeSplitInstruction struct {
	ConnectedAccountID string `json:"connected_account_id"`
	BeneficiaryLabel   string `json:"beneficiary_label"`
	Amount             int64  `json:"amount"`
}

type AuthorizeRiskContext struct {
	DeviceFingerprint string `json:"device_fingerprint"`
	BrowserLanguage   string `json:"browser_language"`
	UserAgent         string `json:"user_agent"`
	CardBIN           string `json:"card_bin"`
	CardNetwork       string `json:"card_network"`
	IssuerCountry     string `json:"issuer_country"`
	CardCountry       string `json:"card_country"`
	FundingType       string `json:"funding_type"`
	MerchantCountry   string `json:"merchant_country"`
}

type AuthorizeRiskInput struct {
	MerchantID  string
	PaymentID   string
	Amount      int64
	Currency    string
	IPAddress   string
	RiskContext AuthorizeRiskContext
}

type CreateUPIIntentInput struct {
	PaymentID        string `json:"-"`
	MerchantID       string `json:"-"`
	OrderID          string `json:"order_id"`
	Amount           int64  `json:"amount"`
	Currency         string `json:"currency"`
	VPA              string `json:"vpa"`
	IdempotencyKey   string `json:"-"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

type CreateUPIQRInput struct {
	PaymentID        string `json:"-"`
	MerchantID       string `json:"-"`
	OrderID          string `json:"order_id"`
	Amount           int64  `json:"amount"`
	Currency         string `json:"currency"`
	QRMode           string `json:"qr_mode"`
	DisplayName      string `json:"display_name"`
	IsReusable       bool   `json:"is_reusable"`
	IdempotencyKey   string `json:"-"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

type CreateUPIMandateChargeInput struct {
	PaymentID      string
	MerchantID     string
	OrderID        string
	Amount         int64
	Currency       string
	MandateID      string
	DisplayName    string
	VPA            string
	IdempotencyKey string
}

type CreateRedirectSessionInput struct {
	PaymentID        string `json:"-"`
	MerchantID       string `json:"-"`
	OrderID          string `json:"order_id"`
	Amount           int64  `json:"amount"`
	Currency         string `json:"currency"`
	Method           string `json:"method"`
	BankCode         string `json:"bank_code"`
	WalletCode       string `json:"wallet_code"`
	IdempotencyKey   string `json:"-"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

type CompleteCardChallengeInput struct {
	MerchantID       string
	PaymentID        string
	CallbackToken    string
	GatewayReference string
	AuthCode         string
	ErrorCode        string
	ErrorDescription string
	Status           string
}

func (s *Service) Authorize(ctx context.Context, in AuthorizeInput) (CaptureResult, error) {
	if in.Amount <= 0 {
		return CaptureResult{}, ErrInvalidPaymentAmount
	}
	if err := validateSplitInstructions(in); err != nil {
		return CaptureResult{}, err
	}
	if in.PaymentID == "" {
		in.PaymentID = idgen.New("pay")
	}

	pending, err := s.repo.StartAuthorization(ctx, CreateAuthorizedInput{
		PaymentID:      in.PaymentID,
		MerchantID:     in.MerchantID,
		OrderID:        in.OrderID,
		Amount:         in.Amount,
		Currency:       in.Currency,
		Method:         in.Method,
		IdempotencyKey: in.IdempotencyKey,
		Splits:         toCreatePaymentSplits(in),
	})
	if err != nil {
		return CaptureResult{}, err
	}
	if pending.Status != StateCreated {
		return pending, nil
	}

	if in.Method == "card" && s.cardTokens != nil {
		if in.PaymentMethodTokenID == "" {
			return CaptureResult{}, ErrPaymentMethodTokenRequired
		}
		cardRef, err := s.cardTokens.PrepareAuthorization(ctx, in.MerchantID, in.PaymentMethodTokenID, in.PaymentID)
		if err != nil {
			return CaptureResult{}, fmt.Errorf("%w: %v", ErrPaymentMethodTokenInvalid, err)
		}
		if err := s.repo.AttachCardPaymentDetails(ctx, in.MerchantID, pending.PaymentID, CardPaymentDetailsInput{
			PaymentMethodTokenID: in.PaymentMethodTokenID,
			CardTokenClass:       string(cardRef.TokenClass),
			CardBrand:            cardRef.Brand,
			CardLast4:            cardRef.Last4,
			CardExpMonth:         cardRef.ExpMonth,
			CardExpYear:          cardRef.ExpYear,
			CardIssuerName:       cardRef.IssuerName,
			CardIssuerCountry:    cardRef.IssuerCountry,
			CardCountry:          cardRef.CardCountry,
			FundingType:          cardRef.FundingType,
			NetworkTokenType:     cardRef.NetworkTokenType,
		}); err != nil {
			return CaptureResult{}, err
		}
		pending.PaymentMethodTokenID = in.PaymentMethodTokenID
		pending.CardBrand = cardRef.Brand
		pending.CardLast4 = cardRef.Last4
		pending.CardExpMonth = cardRef.ExpMonth
		pending.CardExpYear = cardRef.ExpYear
		pending.CardIssuerName = cardRef.IssuerName
		pending.CardIssuerCountry = cardRef.IssuerCountry
		pending.CardCountry = cardRef.CardCountry
		pending.FundingType = cardRef.FundingType
		pending.NetworkTokenType = cardRef.NetworkTokenType
	}

	provider := gatewayProviderName(s.gateway)
	startedAt := time.Now()
	decision := GatewayRouteDecision{
		Provider:           provider,
		RoutingReason:      "direct_gateway",
		AttemptedProviders: []string{provider},
		PrimaryProvider:    provider,
	}
	var result GatewayAuthResult
	if routed, ok := s.gateway.(RoutedGateway); ok {
		result, decision, err = routed.AuthorizeWithRouting(ctx, in.Amount, in.Currency, in.MerchantID, in.Method)
		if decision.Provider == "" {
			decision.Provider = provider
		}
		if len(decision.AttemptedProviders) == 0 {
			decision.AttemptedProviders = []string{decision.Provider}
		}
		if decision.PrimaryProvider == "" {
			decision.PrimaryProvider = decision.Provider
		}
	} else {
		result, err = s.gateway.Authorize(ctx, in.Amount, in.Currency, in.MerchantID, in.Method)
	}
	metrics.GatewayAuthorizationDuration.WithLabelValues(in.Method, provider).Observe(time.Since(startedAt).Seconds())
	if err != nil {
		metrics.GatewayAuthorizationsTotal.WithLabelValues(in.Method, provider, "error").Inc()
		err = s.persistAuthorizationFailure(ctx, pending, decision, "GATEWAY_ERROR", err.Error(), "gateway error", err)
		return CaptureResult{}, fmt.Errorf("gateway authorize: %w", err)
	}
	if result.RequiresAction {
		_ = s.repo.UpdateGatewayRouting(ctx, in.MerchantID, pending.PaymentID, decision)
		metrics.GatewayAuthorizationsTotal.WithLabelValues(in.Method, provider, "requires_action").Inc()
		out, err := s.repo.MarkAuthorizationRequiresAction(ctx, in.MerchantID, pending.PaymentID, CardChallengeSessionInput{
			SessionID:            idgen.New("chlg"),
			CardTokenID:          in.PaymentMethodTokenID,
			RedirectURL:          result.ChallengeURL,
			ChallengeReference:   result.ChallengeToken,
			CallbackToken:        idgen.New("chcb"),
			AutoCaptureRequested: in.AutoCapture,
			ExpiresAt:            derefTime(result.ChallengeExpiry, time.Now().UTC().Add(5*time.Minute)),
		})
		if err != nil {
			if in.Method == "card" && s.cardTokens != nil && pending.PaymentMethodTokenID != "" {
				_ = s.cardTokens.CompleteAuthorization(ctx, in.MerchantID, pending.PaymentMethodTokenID, pending.PaymentID, false, "challenge persist failed")
			}
			return CaptureResult{}, err
		}
		return out, nil
	}
	if !result.Success {
		metrics.GatewayAuthorizationsTotal.WithLabelValues(in.Method, provider, "declined").Inc()
		err = s.persistAuthorizationFailure(ctx, pending, decision, result.ErrorCode, result.ErrorDescription, result.ErrorDescription, ErrAuthorizationDeclined)
		return CaptureResult{}, err
	}

	var autoCaptureAt *time.Time
	if in.AutoCapture {
		t := time.Now().UTC().Add(30 * time.Second)
		autoCaptureAt = &t
	}
	out, err := s.repo.MarkAuthorizationAuthorized(ctx, in.MerchantID, pending.PaymentID, result.GatewayReference, result.AuthCode, autoCaptureAt)
	if err != nil {
		if in.Method == "card" && s.cardTokens != nil && pending.PaymentMethodTokenID != "" {
			_ = s.cardTokens.CompleteAuthorization(ctx, in.MerchantID, pending.PaymentMethodTokenID, pending.PaymentID, false, "authorization persist failed")
		}
		return CaptureResult{}, err
	}
	_ = s.repo.UpdateGatewayRouting(ctx, in.MerchantID, pending.PaymentID, decision)
	if in.Method == "card" && s.cardTokens != nil && pending.PaymentMethodTokenID != "" {
		_ = s.cardTokens.CompleteAuthorization(ctx, in.MerchantID, pending.PaymentMethodTokenID, pending.PaymentID, true, "")
	}
	metrics.GatewayAuthorizationsTotal.WithLabelValues(in.Method, provider, "authorized").Inc()
	return out, nil
}

func gatewayProviderName(gateway GatewayClient) string {
	if named, ok := gateway.(NamedGateway); ok {
		if name := named.Name(); name != "" {
			return name
		}
	}
	return "unknown"
}

func (s *Service) RecordFailedAttempt(ctx context.Context, in AuthorizeInput, errorCode, errorDescription string) error {
	if in.Amount <= 0 {
		return ErrInvalidPaymentAmount
	}
	if in.PaymentID == "" {
		in.PaymentID = idgen.New("pay")
	}
	return s.repo.CreateFailedAttempt(ctx, CreateAuthorizedInput{
		PaymentID:      in.PaymentID,
		MerchantID:     in.MerchantID,
		OrderID:        in.OrderID,
		Amount:         in.Amount,
		Currency:       in.Currency,
		Method:         in.Method,
		IdempotencyKey: in.IdempotencyKey,
	}, errorCode, errorDescription)
}

func (s *Service) CreateUPIIntent(ctx context.Context, in CreateUPIIntentInput) (UPIIntentResult, error) {
	if s.upi == nil {
		return UPIIntentResult{}, errors.New("upi intent gateway is not configured")
	}
	if in.Amount <= 0 {
		return UPIIntentResult{}, ErrInvalidPaymentAmount
	}
	normalizedVPA, err := upiverify.NormalizeVPA(in.VPA)
	if err != nil {
		return UPIIntentResult{}, ErrInvalidUPIVPA
	}
	if in.PaymentID == "" {
		in.PaymentID = idgen.New("pay")
	}
	expiresIn := in.ExpiresInSeconds
	if expiresIn <= 0 {
		expiresIn = 300
	}
	expiresAt := time.Now().UTC().Add(time.Duration(expiresIn) * time.Second)
	callbackToken := idgen.New("upicb")

	intent, err := s.repo.CreateUPIIntent(ctx, CreateUPIIntentRecordInput{
		PaymentID:      in.PaymentID,
		MerchantID:     in.MerchantID,
		OrderID:        in.OrderID,
		Amount:         in.Amount,
		Currency:       in.Currency,
		Method:         "upi",
		IdempotencyKey: in.IdempotencyKey,
		FlowType:       "intent",
		VPA:            normalizedVPA,
		ExpiresAt:      expiresAt,
		CallbackToken:  callbackToken,
	})
	if err != nil {
		return UPIIntentResult{}, err
	}
	if intent.DeepLink != "" {
		return intent, nil
	}

	gatewayResult, err := s.upi.CreateUPIIntent(ctx, GatewayUPIIntentCreateInput{
		PaymentID:  intent.PaymentID,
		MerchantID: intent.MerchantID,
		OrderID:    intent.OrderID,
		Amount:     intent.Amount,
		Currency:   intent.Currency,
		FlowType:   "intent",
		VPA:        intent.VPA,
		ExpiresAt:  intent.ExpiresAt,
	})
	if err != nil {
		err = s.persistUPIIntentFailure(ctx, intent.PaymentID, "UPI_GATEWAY_ERROR", err.Error(), err)
		return UPIIntentResult{}, fmt.Errorf("create upi intent: %w", err)
	}
	return s.repo.AttachUPIIntentGatewayData(ctx, intent.MerchantID, intent.PaymentID, gatewayResult)
}

func (s *Service) CreateUPIQR(ctx context.Context, in CreateUPIQRInput) (UPIIntentResult, error) {
	if s.upi == nil {
		return UPIIntentResult{}, errors.New("upi intent gateway is not configured")
	}
	if in.Amount <= 0 {
		return UPIIntentResult{}, ErrInvalidPaymentAmount
	}
	if in.PaymentID == "" {
		in.PaymentID = idgen.New("pay")
	}
	if in.QRMode == "" {
		in.QRMode = "dynamic"
	}
	expiresIn := in.ExpiresInSeconds
	if expiresIn <= 0 {
		expiresIn = 300
	}
	expiresAt := time.Now().UTC().Add(time.Duration(expiresIn) * time.Second)
	callbackToken := idgen.New("upicb")

	intent, err := s.repo.CreateUPIIntent(ctx, CreateUPIIntentRecordInput{
		PaymentID:      in.PaymentID,
		MerchantID:     in.MerchantID,
		OrderID:        in.OrderID,
		Amount:         in.Amount,
		Currency:       in.Currency,
		Method:         "upi",
		IdempotencyKey: in.IdempotencyKey,
		FlowType:       "qr",
		QRMode:         in.QRMode,
		DisplayName:    in.DisplayName,
		IsReusable:     in.IsReusable,
		ExpiresAt:      expiresAt,
		CallbackToken:  callbackToken,
	})
	if err != nil {
		return UPIIntentResult{}, err
	}
	if intent.QRPayload != "" {
		return intent, nil
	}

	gatewayResult, err := s.upi.CreateUPIIntent(ctx, GatewayUPIIntentCreateInput{
		PaymentID:   intent.PaymentID,
		MerchantID:  intent.MerchantID,
		OrderID:     intent.OrderID,
		Amount:      intent.Amount,
		Currency:    intent.Currency,
		FlowType:    "qr",
		QRMode:      intent.QRMode,
		QRPayload:   intent.QRPayload,
		QRImageURL:  intent.QRImageURL,
		IsReusable:  intent.IsReusable,
		DisplayName: intent.DisplayName,
		ExpiresAt:   intent.ExpiresAt,
	})
	if err != nil {
		err = s.persistUPIIntentFailure(ctx, intent.PaymentID, "UPI_GATEWAY_ERROR", err.Error(), err)
		return UPIIntentResult{}, fmt.Errorf("create upi qr: %w", err)
	}
	return s.repo.AttachUPIIntentGatewayData(ctx, intent.MerchantID, intent.PaymentID, gatewayResult)
}

func (s *Service) CreateUPIMandateCharge(ctx context.Context, in CreateUPIMandateChargeInput) (UPIIntentResult, error) {
	if s.upi == nil {
		return UPIIntentResult{}, errors.New("upi intent gateway is not configured")
	}
	if in.Amount <= 0 {
		return UPIIntentResult{}, ErrInvalidPaymentAmount
	}
	if in.PaymentID == "" {
		in.PaymentID = idgen.New("pay")
	}
	callbackToken := idgen.New("upicb")
	expiresAt := time.Now().UTC().Add(5 * time.Minute)

	intent, err := s.repo.CreateUPIIntent(ctx, CreateUPIIntentRecordInput{
		PaymentID:      in.PaymentID,
		MerchantID:     in.MerchantID,
		OrderID:        in.OrderID,
		Amount:         in.Amount,
		Currency:       in.Currency,
		Method:         "upi",
		IdempotencyKey: in.IdempotencyKey,
		FlowType:       "mandate",
		MandateID:      in.MandateID,
		DisplayName:    in.DisplayName,
		VPA:            in.VPA,
		ExpiresAt:      expiresAt,
		CallbackToken:  callbackToken,
	})
	if err != nil {
		return UPIIntentResult{}, err
	}
	gatewayResult, err := s.upi.CreateUPIIntent(ctx, GatewayUPIIntentCreateInput{
		PaymentID:   intent.PaymentID,
		MerchantID:  intent.MerchantID,
		OrderID:     intent.OrderID,
		Amount:      intent.Amount,
		Currency:    intent.Currency,
		FlowType:    "mandate",
		MandateID:   intent.MandateID,
		DisplayName: intent.DisplayName,
		VPA:         intent.VPA,
		ExpiresAt:   intent.ExpiresAt,
	})
	if err != nil {
		err = s.persistUPIIntentFailure(ctx, intent.PaymentID, "UPI_MANDATE_GATEWAY_ERROR", err.Error(), err)
		return UPIIntentResult{}, fmt.Errorf("create upi mandate charge: %w", err)
	}
	intent, err = s.repo.AttachUPIIntentGatewayData(ctx, intent.MerchantID, intent.PaymentID, gatewayResult)
	if err != nil {
		return UPIIntentResult{}, err
	}
	out, _, err := s.repo.CompleteUPIIntent(ctx, intent.PaymentID, "", gatewayResult.GatewayReference, time.Now().UTC())
	return out, err
}

func (s *Service) GetUPIIntent(ctx context.Context, merchantID, paymentID string) (UPIIntentResult, error) {
	return s.repo.GetUPIIntent(ctx, merchantID, paymentID)
}

func (s *Service) PollUPIIntent(ctx context.Context, merchantID, paymentID string) (UPIIntentResult, error) {
	if s.upi == nil {
		return UPIIntentResult{}, errors.New("upi intent gateway is not configured")
	}
	intent, err := s.repo.GetUPIIntent(ctx, merchantID, paymentID)
	if err != nil {
		return UPIIntentResult{}, err
	}
	if time.Now().UTC().After(intent.ExpiresAt) && (intent.Status == StatePendingCustomerAction || intent.Status == StateProcessing) {
		return s.repo.ExpireUPIIntent(ctx, merchantID, paymentID, time.Now().UTC())
	}
	status, err := s.upi.PollUPIIntent(ctx, GatewayUPIIntentPollInput{
		PaymentID:        intent.PaymentID,
		MerchantID:       intent.MerchantID,
		OrderID:          intent.OrderID,
		GatewayReference: intent.GatewayReference,
		CreatedAt:        intent.CreatedAt,
		ExpiresAt:        intent.ExpiresAt,
	})
	if err != nil {
		return UPIIntentResult{}, fmt.Errorf("poll upi intent: %w", err)
	}
	if err := s.repo.PollUPIIntent(ctx, merchantID, paymentID, time.Now().UTC()); err != nil {
		return UPIIntentResult{}, err
	}
	switch status.ProviderStatus {
	case UPIProviderStatusPending:
		if intent.Status == StatePendingCustomerAction {
			out, _, err := s.repo.MarkUPIIntentProcessing(ctx, paymentID, "")
			return out, err
		}
		return s.repo.GetUPIIntent(ctx, merchantID, paymentID)
	case UPIProviderStatusSucceeded:
		out, _, err := s.repo.CompleteUPIIntent(ctx, paymentID, "", status.GatewayReference, time.Now().UTC())
		return out, err
	case UPIProviderStatusFailed:
		out, _, err := s.repo.FailUPIIntent(ctx, paymentID, "", UPIProviderStatusFailed, status.ErrorCode, status.ErrorDescription, time.Now().UTC())
		return out, err
	default:
		return s.repo.GetUPIIntent(ctx, merchantID, paymentID)
	}
}

func (s *Service) ApplyUPICallback(ctx context.Context, merchantID, paymentID, callbackToken, eventID, status, gatewayReference, errorCode, errorDescription string) (UPIIntentResult, bool, error) {
	intent, err := s.repo.GetUPIIntent(ctx, merchantID, paymentID)
	if err != nil {
		return UPIIntentResult{}, false, err
	}
	if callbackToken == "" || callbackToken != intent.CallbackToken {
		return UPIIntentResult{}, false, ErrUPICallbackRejected
	}
	now := time.Now().UTC()
	switch status {
	case "processing":
		return s.repo.MarkUPIIntentProcessing(ctx, paymentID, eventID)
	case "succeeded":
		return s.repo.CompleteUPIIntent(ctx, paymentID, eventID, gatewayReference, now)
	case "failed":
		return s.repo.FailUPIIntent(ctx, paymentID, eventID, UPIProviderStatusFailed, errorCode, errorDescription, now)
	case "abandoned":
		out, err := s.repo.AbandonUPIIntent(ctx, merchantID, paymentID, errorDescription, now)
		return out, true, err
	case "expired":
		out, err := s.repo.ExpireUPIIntent(ctx, merchantID, paymentID, now)
		return out, true, err
	default:
		return UPIIntentResult{}, false, ErrUPICallbackRejected
	}
}

func (s *Service) AbandonUPIIntent(ctx context.Context, merchantID, paymentID, reason string) (UPIIntentResult, error) {
	return s.repo.AbandonUPIIntent(ctx, merchantID, paymentID, reason, time.Now().UTC())
}

func (s *Service) CaptureForMerchant(ctx context.Context, merchantID, paymentID string, amount int64) (CaptureResult, error) {
	if merchantID == "" {
		return CaptureResult{}, errors.New("merchant id is required")
	}
	return s.repo.CaptureAuthorizedPayment(ctx, merchantID, paymentID, amount)
}

func (s *Service) ReverseAuthorization(ctx context.Context, merchantID, paymentID, reason string) (CaptureResult, error) {
	return s.repo.ReverseAuthorization(ctx, merchantID, paymentID, reason)
}

func (s *Service) persistAuthorizationFailure(ctx context.Context, pending CaptureResult, decision GatewayRouteDecision, errorCode, errorDescription, tokenReason string, primaryErr error) error {
	var cleanupErrs []error
	if err := s.repo.UpdateGatewayRouting(ctx, pending.MerchantID, pending.PaymentID, decision); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("update gateway routing: %w", err))
	}
	if pending.Method == "card" && s.cardTokens != nil && pending.PaymentMethodTokenID != "" {
		if err := s.cardTokens.CompleteAuthorization(ctx, pending.MerchantID, pending.PaymentMethodTokenID, pending.PaymentID, false, tokenReason); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("complete card token authorization: %w", err))
		}
	}
	if err := s.repo.MarkAuthorizationFailed(ctx, pending.MerchantID, pending.PaymentID, errorCode, errorDescription); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("mark authorization failed: %w", err))
	}
	if len(cleanupErrs) == 0 {
		return primaryErr
	}
	return errors.Join(append([]error{primaryErr}, cleanupErrs...)...)
}

func (s *Service) persistUPIIntentFailure(ctx context.Context, paymentID, errorCode, errorDescription string, primaryErr error) error {
	if _, _, err := s.repo.FailUPIIntent(ctx, paymentID, "", UPIProviderStatusFailed, errorCode, errorDescription, time.Now().UTC()); err != nil {
		return errors.Join(primaryErr, fmt.Errorf("persist upi intent failure state: %w", err))
	}
	return primaryErr
}

func (s *Service) CompleteCardChallenge(ctx context.Context, in CompleteCardChallengeInput) (CaptureResult, bool, error) {
	if in.Status == "" {
		in.Status = "succeeded"
	}
	current, err := s.repo.GetPayment(ctx, in.MerchantID, in.PaymentID)
	if err != nil {
		return CaptureResult{}, false, err
	}
	switch in.Status {
	case "succeeded":
		out, processed, err := s.repo.CompleteCardChallenge(ctx, in.MerchantID, in.PaymentID, in.CallbackToken, in.GatewayReference, in.AuthCode, time.Now().UTC())
		if processed && s.cardTokens != nil && current.PaymentMethodTokenID != "" {
			_ = s.cardTokens.CompleteAuthorization(ctx, in.MerchantID, current.PaymentMethodTokenID, in.PaymentID, true, "")
		}
		return out, processed, err
	case "abandoned", "expired", "failed":
		status := "failed"
		reason := in.ErrorDescription
		if in.Status == "abandoned" {
			status = "abandoned"
			if reason == "" {
				reason = "customer abandoned challenge"
			}
		}
		if in.Status == "expired" {
			status = "expired"
			if reason == "" {
				reason = "challenge session expired"
			}
		}
		out, processed, err := s.repo.FailCardChallenge(ctx, in.MerchantID, in.PaymentID, in.CallbackToken, firstNonEmpty(in.ErrorCode, "CARD_AUTHENTICATION_FAILED"), reason, status, time.Now().UTC())
		if processed && s.cardTokens != nil && current.PaymentMethodTokenID != "" {
			_ = s.cardTokens.CompleteAuthorization(ctx, in.MerchantID, current.PaymentMethodTokenID, in.PaymentID, false, reason)
		}
		return out, processed, err
	default:
		return CaptureResult{}, false, ErrInvalidTransition
	}
}

func (s *Service) Get(ctx context.Context, merchantID, paymentID string) (CaptureResult, error) {
	return s.repo.GetPayment(ctx, merchantID, paymentID)
}

func (s *Service) List(ctx context.Context, f ListFilter) (ListResult, error) {
	if f.Count <= 0 || f.Count > 100 {
		f.Count = 20
	}
	return s.repo.ListPayments(ctx, f)
}

func toCreatePaymentSplits(in AuthorizeInput) []CreatePaymentSplitInput {
	if len(in.SplitInstructions) == 0 {
		return nil
	}
	out := make([]CreatePaymentSplitInput, 0, len(in.SplitInstructions))
	for _, item := range in.SplitInstructions {
		out = append(out, CreatePaymentSplitInput{
			DestinationType:  "connected_account",
			DestinationRef:   item.ConnectedAccountID,
			BeneficiaryLabel: item.BeneficiaryLabel,
			Amount:           item.Amount,
			Currency:         in.Currency,
		})
	}
	return out
}

func validateSplitInstructions(in AuthorizeInput) error {
	if len(in.SplitInstructions) == 0 {
		return nil
	}
	fee := in.Amount * 2 / 100
	var total int64
	for _, item := range in.SplitInstructions {
		if item.Amount < 0 || item.ConnectedAccountID == "" {
			return ErrAmountMismatch
		}
		total += item.Amount
	}
	if total > in.Amount-fee {
		return ErrAmountMismatch
	}
	return nil
}

func derefTime(v *time.Time, fallback time.Time) time.Time {
	if v != nil {
		return *v
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
