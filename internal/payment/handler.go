package payment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/common/middleware"
	"github.com/sanskarpan/PayGate/internal/merchant"
)

// RiskEvaluator is an optional hook called before gateway authorization to
// decide whether a payment should proceed, be held, or be blocked.
type RiskEvaluator interface {
	EvaluateAuthorize(ctx context.Context, in AuthorizeRiskInput) (string, error)
}

type CapabilityChecker interface {
	CheckCapability(ctx context.Context, merchantID string, capability merchant.CapabilityCode) error
}

type Handler struct {
	svc          *Service
	risk         RiskEvaluator // optional
	capabilities CapabilityChecker
}

func NewHandler(svc *Service, opts ...func(*Handler)) *Handler {
	h := &Handler{svc: svc}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// WithRiskEvaluator attaches an optional risk evaluator to the payment handler.
func WithRiskEvaluator(r RiskEvaluator) func(*Handler) {
	return func(h *Handler) { h.risk = r }
}

func WithCapabilityChecker(c CapabilityChecker) func(*Handler) {
	return func(h *Handler) { h.capabilities = c }
}

func (h *Handler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(scope merchant.APIKeyScope, next http.Handler) http.Handler) {
	mux.Handle("POST /v1/payments/upi/intents", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.createUPIIntent)))
	mux.Handle("GET /v1/payments/{paymentID}/upi-intent", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getUPIIntent)))
	mux.Handle("POST /v1/payments/{paymentID}/upi-intent/poll", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.pollUPIIntent)))
	mux.Handle("POST /v1/payments/{paymentID}/upi-intent/abandon", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.abandonUPIIntent)))
	mux.Handle("POST /v1/payments/authorize", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.authorize)))
	mux.Handle("POST /v1/payments/{paymentID}/capture", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.capture)))
	mux.Handle("POST /v1/payments/{paymentID}/reverse-authorization", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.reverseAuthorization)))
	mux.Handle("GET /v1/payments/{paymentID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.get)))
	mux.Handle("GET /v1/payments", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.list)))
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /v1/payments/{paymentID}/upi-intent/callbacks", http.HandlerFunc(h.upiCallback))
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req AuthorizeInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	req.MerchantID = p.MerchantID
	req.IdempotencyKey = r.Header.Get("Idempotency-Key")
	if req.Currency == "" {
		req.Currency = "INR"
	}
	if req.Method == "" {
		req.Method = "card"
	}
	if h.capabilities != nil {
		if err := h.capabilities.CheckCapability(r.Context(), p.MerchantID, merchant.CapabilityPayments); err != nil {
			handleError(w, err)
			return
		}
		if req.Method == "upi" {
			if err := h.capabilities.CheckCapability(r.Context(), p.MerchantID, merchant.CapabilityUPI); err != nil {
				handleError(w, err)
				return
			}
		}
		if req.Method == "card" {
			if err := h.capabilities.CheckCapability(r.Context(), p.MerchantID, merchant.CapabilityCards); err != nil {
				handleError(w, err)
				return
			}
		}
	}
	req.PaymentID = idgen.New("pay")

	// Evaluate risk before contacting the gateway so blocked payments never
	// persist as authorized or get swept into auto-capture.
	if h.risk != nil {
		ip := middleware.ClientIPWithTrustedProxies(r, nil)
		riskContext := AuthorizeRiskContext{
			BrowserLanguage: r.Header.Get("Accept-Language"),
			UserAgent:       r.UserAgent(),
		}
		if req.RiskContext != nil {
			riskContext = *req.RiskContext
			if riskContext.BrowserLanguage == "" {
				riskContext.BrowserLanguage = r.Header.Get("Accept-Language")
			}
			if riskContext.UserAgent == "" {
				riskContext.UserAgent = r.UserAgent()
			}
		}
		action, riskErr := h.risk.EvaluateAuthorize(r.Context(), AuthorizeRiskInput{
			MerchantID:  p.MerchantID,
			PaymentID:   req.PaymentID,
			Amount:      req.Amount,
			Currency:    req.Currency,
			IPAddress:   ip,
			RiskContext: riskContext,
		})
		if riskErr == nil && action == "block" {
			_ = h.svc.RecordFailedAttempt(r.Context(), req, "RISK_BLOCKED", "payment blocked due to risk policy")
			httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.APIError{
				Code:        "RISK_BLOCKED",
				Description: "payment blocked due to risk policy",
				Source:      "risk",
				Step:        "payment_authorization",
				Reason:      "risk_score_exceeded",
			})
			return
		}
		if riskErr == nil && action == "hold" {
			req.AutoCapture = false
		}
	}

	out, err := h.svc.Authorize(r.Context(), req)
	if err != nil {
		handleError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, present(out))
}

func (h *Handler) createUPIIntent(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req CreateUPIIntentInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	req.MerchantID = p.MerchantID
	if h.capabilities != nil {
		if err := h.capabilities.CheckCapability(r.Context(), p.MerchantID, merchant.CapabilityPayments); err != nil {
			handleError(w, err)
			return
		}
		if err := h.capabilities.CheckCapability(r.Context(), p.MerchantID, merchant.CapabilityUPI); err != nil {
			handleError(w, err)
			return
		}
	}
	req.IdempotencyKey = r.Header.Get("Idempotency-Key")
	if req.Currency == "" {
		req.Currency = "INR"
	}
	out, err := h.svc.CreateUPIIntent(r.Context(), req)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentUPIIntent(out, callbackURL(r, out.PaymentID, out.CallbackToken)))
}

func (h *Handler) getUPIIntent(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.GetUPIIntent(r.Context(), p.MerchantID, r.PathValue("paymentID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentUPIIntent(out, callbackURL(r, out.PaymentID, out.CallbackToken)))
}

func (h *Handler) pollUPIIntent(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.PollUPIIntent(r.Context(), p.MerchantID, r.PathValue("paymentID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentUPIIntent(out, callbackURL(r, out.PaymentID, out.CallbackToken)))
}

func (h *Handler) abandonUPIIntent(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
			return
		}
	}
	out, err := h.svc.AbandonUPIIntent(r.Context(), p.MerchantID, r.PathValue("paymentID"), req.Reason)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentUPIIntent(out, callbackURL(r, out.PaymentID, out.CallbackToken)))
}

func (h *Handler) upiCallback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MerchantID       string `json:"merchant_id"`
		EventID          string `json:"event_id"`
		Status           string `json:"status"`
		GatewayReference string `json:"gateway_reference"`
		ErrorCode        string `json:"error_code"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	out, processed, err := h.svc.ApplyUPICallback(
		r.Context(),
		req.MerchantID,
		r.PathValue("paymentID"),
		r.URL.Query().Get("token"),
		req.EventID,
		req.Status,
		req.GatewayReference,
		req.ErrorCode,
		req.ErrorDescription,
	)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"entity":    "upi_callback_result",
		"processed": processed,
		"payment":   presentUPIIntent(out, callbackURL(r, out.PaymentID, out.CallbackToken)),
	})
}

func (h *Handler) capture(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Amount int64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	out, err := h.svc.CaptureForMerchant(r.Context(), p.MerchantID, r.PathValue("paymentID"), req.Amount)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(out))
}

func (h *Handler) reverseAuthorization(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
			return
		}
	}
	out, err := h.svc.ReverseAuthorization(r.Context(), p.MerchantID, r.PathValue("paymentID"), req.Reason)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(out))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.Get(r.Context(), p.MerchantID, r.PathValue("paymentID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(out))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	out, err := h.svc.List(r.Context(), ListFilter{
		MerchantID: p.MerchantID,
		OrderID:    r.URL.Query().Get("order_id"),
		Count:      count,
	})
	if err != nil {
		handleError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(out.Items))
	for _, item := range out.Items {
		items = append(items, present(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity": "collection",
		"count":  len(items),
		"items":  items,
	})
}

func present(out CaptureResult) map[string]any {
	var capturedAt int64
	if out.CapturedAt != nil {
		capturedAt = out.CapturedAt.Unix()
	}
	var authorizedAt int64
	if out.AuthorizedAt != nil {
		authorizedAt = out.AuthorizedAt.Unix()
	}
	resp := map[string]any{
		"id":                      out.PaymentID,
		"entity":                  "payment",
		"amount":                  out.Amount,
		"currency":                out.Currency,
		"status":                  out.Status,
		"order_id":                out.OrderID,
		"method":                  out.Method,
		"method_state":            out.MethodState,
		"method_state_reason":     out.MethodStateReason,
		"captured":                out.Captured,
		"captured_at":             capturedAt,
		"authorized_at":           authorizedAt,
		"created_at":              out.CreatedAt.Unix(),
		"payment_method_token_id": out.PaymentMethodTokenID,
		"card": map[string]any{
			"brand":     out.CardBrand,
			"last4":     out.CardLast4,
			"exp_month": out.CardExpMonth,
			"exp_year":  out.CardExpYear,
		},
	}
	if len(out.Splits) > 0 {
		splits := make([]map[string]any, 0, len(out.Splits))
		for _, split := range out.Splits {
			splits = append(splits, map[string]any{
				"destination_type":  split.DestinationType,
				"destination_ref":   split.DestinationRef,
				"beneficiary_label": split.BeneficiaryLabel,
				"amount":            split.Amount,
				"currency":          split.Currency,
			})
		}
		resp["splits"] = splits
	}
	return resp
}

func presentUPIIntent(out UPIIntentResult, callbackURL string) map[string]any {
	resp := present(out.CaptureResult)
	resp["entity"] = "upi_intent_payment"
	resp["vpa"] = out.VPA
	resp["provider_status"] = out.ProviderStatus
	resp["gateway_reference"] = out.GatewayReference
	resp["expires_at"] = out.ExpiresAt.Unix()
	resp["failure_code"] = out.FailureCode
	resp["failure_description"] = out.FailureDescription
	if out.CompletedAt != nil {
		resp["completed_at"] = out.CompletedAt.Unix()
	}
	if out.LastPolledAt != nil {
		resp["last_polled_at"] = out.LastPolledAt.Unix()
	}
	resp["next_action"] = map[string]any{
		"type":       "upi_intent",
		"deep_link":  out.DeepLink,
		"intent_uri": out.IntentURI,
		"expires_at": out.ExpiresAt.Unix(),
	}
	resp["sandbox"] = map[string]any{
		"callback_url": callbackURL,
	}
	return resp
}

func callbackURL(r *http.Request, paymentID, token string) string {
	if paymentID == "" || token == "" {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/v1/payments/" + paymentID + "/upi-intent/callbacks?token=" + token
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrPaymentNotFound), errors.Is(err, ErrOrderNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrOrderExpired),
		errors.Is(err, ErrCurrencyMismatch),
		errors.Is(err, ErrAmountMismatch),
		errors.Is(err, ErrInvalidPaymentAmount),
		errors.Is(err, ErrPaymentMethodTokenRequired),
		errors.Is(err, ErrPaymentMethodTokenInvalid):
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: err.Error()})
	case errors.Is(err, ErrUPICallbackRejected):
		httpx.WriteError(w, http.StatusForbidden, httpx.APIError{Code: "FORBIDDEN", Description: err.Error()})
	case errors.Is(err, ErrUPIIntentNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrInvalidTransition):
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: err.Error()})
	case errors.Is(err, ErrAuthorizationDeclined):
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.APIError{Code: "PAYMENT_FAILED", Description: err.Error()})
	case errors.Is(err, merchant.ErrCapabilityRestricted):
		httpx.WriteError(w, http.StatusForbidden, httpx.APIError{Code: "CAPABILITY_RESTRICTED", Description: err.Error()})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error", Metadata: map[string]any{"at": time.Now().UTC().Unix()}})
	}
}
