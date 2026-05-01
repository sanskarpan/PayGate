package payment

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/merchant"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(scope merchant.APIKeyScope, next http.Handler) http.Handler) {
	mux.Handle("POST /v1/payments/authorize", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.authorize)))
	mux.Handle("POST /v1/payments/{paymentID}/capture", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.capture)))
	mux.Handle("GET /v1/payments/{paymentID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.get)))
	mux.Handle("GET /v1/payments", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.list)))
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
	out, err := h.svc.Authorize(r.Context(), req)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, present(out))
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
	return map[string]any{
		"id":            out.PaymentID,
		"entity":        "payment",
		"amount":        out.Amount,
		"currency":      out.Currency,
		"status":        out.Status,
		"order_id":      out.OrderID,
		"method":        out.Method,
		"captured":      out.Captured,
		"captured_at":   capturedAt,
		"authorized_at": authorizedAt,
		"created_at":    out.CreatedAt.Unix(),
	}
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrPaymentNotFound), errors.Is(err, ErrOrderNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrOrderExpired),
		errors.Is(err, ErrCurrencyMismatch),
		errors.Is(err, ErrAmountMismatch),
		errors.Is(err, ErrInvalidPaymentAmount):
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: err.Error()})
	case errors.Is(err, ErrInvalidTransition):
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: err.Error()})
	case errors.Is(err, ErrAuthorizationDeclined):
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.APIError{Code: "PAYMENT_FAILED", Description: err.Error()})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error", Metadata: map[string]any{"at": time.Now().UTC().Unix()}})
	}
}
