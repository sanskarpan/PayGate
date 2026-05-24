package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/tokenization"
)

var ErrPaymentNotFound = errors.New("payment not found")
var ErrPaymentMethodTokenRequired = errors.New("payment_method_token_id is required for card payments")
var ErrPaymentMethodTokenInvalid = errors.New("payment method token is invalid or unavailable")

type GatewayClient interface {
	Authorize(ctx context.Context, amount int64, currency, merchantID, method string) (GatewayAuthResult, error)
}

type UPIGateway interface {
	CreateUPIIntent(ctx context.Context, in GatewayUPIIntentCreateInput) (GatewayUPIIntentResult, error)
	PollUPIIntent(ctx context.Context, in GatewayUPIIntentPollInput) (GatewayUPIIntentStatus, error)
}

type CardTokenAuthorizer interface {
	PrepareAuthorization(ctx context.Context, merchantID, tokenID, paymentID string) (tokenization.CardTokenReference, error)
}

type GatewayAuthResult struct {
	Success          bool
	GatewayReference string
	AuthCode         string
	ErrorCode        string
	ErrorDescription string
}

type GatewayUPIIntentCreateInput struct {
	PaymentID  string
	MerchantID string
	OrderID    string
	Amount     int64
	Currency   string
	VPA        string
	ExpiresAt  time.Time
}

type GatewayUPIIntentResult struct {
	GatewayReference string
	DeepLink         string
	IntentURI        string
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

type Service struct {
	repo       Repository
	gateway    GatewayClient
	upi        UPIGateway
	cardTokens CardTokenAuthorizer
}

func NewService(repo Repository, gw GatewayClient, opts ...func(*Service)) *Service {
	svc := &Service{repo: repo, gateway: gw}
	if upi, ok := gw.(UPIGateway); ok {
		svc.upi = upi
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
		}); err != nil {
			return CaptureResult{}, err
		}
		pending.PaymentMethodTokenID = in.PaymentMethodTokenID
		pending.CardBrand = cardRef.Brand
		pending.CardLast4 = cardRef.Last4
		pending.CardExpMonth = cardRef.ExpMonth
		pending.CardExpYear = cardRef.ExpYear
	}

	result, err := s.gateway.Authorize(ctx, in.Amount, in.Currency, in.MerchantID, in.Method)
	if err != nil {
		_ = s.repo.MarkAuthorizationFailed(ctx, in.MerchantID, pending.PaymentID, "GATEWAY_ERROR", err.Error())
		return CaptureResult{}, fmt.Errorf("gateway authorize: %w", err)
	}
	if !result.Success {
		_ = s.repo.MarkAuthorizationFailed(ctx, in.MerchantID, pending.PaymentID, result.ErrorCode, result.ErrorDescription)
		return CaptureResult{}, ErrAuthorizationDeclined
	}

	var autoCaptureAt *time.Time
	if in.AutoCapture {
		t := time.Now().UTC().Add(30 * time.Second)
		autoCaptureAt = &t
	}

	return s.repo.MarkAuthorizationAuthorized(ctx, in.MerchantID, pending.PaymentID, result.GatewayReference, result.AuthCode, autoCaptureAt)
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
		VPA:            in.VPA,
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
		VPA:        intent.VPA,
		ExpiresAt:  intent.ExpiresAt,
	})
	if err != nil {
		_, _, _ = s.repo.FailUPIIntent(ctx, intent.PaymentID, "", UPIProviderStatusFailed, "UPI_GATEWAY_ERROR", err.Error(), time.Now().UTC())
		return UPIIntentResult{}, fmt.Errorf("create upi intent: %w", err)
	}
	return s.repo.AttachUPIIntentGatewayData(ctx, intent.MerchantID, intent.PaymentID, gatewayResult)
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
