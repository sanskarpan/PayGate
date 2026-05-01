package refund

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/merchant"
)

// Handler exposes the refund HTTP endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutesWithAuth wires the refund endpoints into mux under auth.
func (h *Handler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(scope merchant.APIKeyScope, next http.Handler) http.Handler) {
	mux.Handle("POST /v1/payments/{paymentID}/refunds", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.create)))
	mux.Handle("GET /v1/refunds/{refundID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.get)))
	mux.Handle("GET /v1/payments/{paymentID}/refunds", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listByPayment)))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Amount int64          `json:"amount"`
		Reason string         `json:"reason"`
		Notes  map[string]any `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	ref, err := h.svc.Initiate(r.Context(), CreateInput{
		PaymentID:  r.PathValue("paymentID"),
		MerchantID: p.MerchantID,
		Amount:     req.Amount,
		Reason:     req.Reason,
		Notes:      req.Notes,
	})
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, present(ref))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	ref, err := h.svc.Get(r.Context(), p.MerchantID, r.PathValue("refundID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(ref))
}

func (h *Handler) listByPayment(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	refs, err := h.svc.ListByPayment(r.Context(), p.MerchantID, r.PathValue("paymentID"))
	if err != nil {
		handleError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		items = append(items, present(ref))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity": "collection",
		"count":  len(items),
		"items":  items,
	})
}

func present(ref Refund) map[string]any {
	var processedAt int64
	if ref.ProcessedAt != nil {
		processedAt = ref.ProcessedAt.Unix()
	}
	return map[string]any{
		"id":          ref.ID,
		"entity":      "refund",
		"payment_id":  ref.PaymentID,
		"amount":      ref.Amount,
		"currency":    ref.Currency,
		"reason":      ref.Reason,
		"status":      ref.Status,
		"notes":       ref.Notes,
		"processed_at": processedAt,
		"created_at":  ref.CreatedAt.Unix(),
		"created_at_rfc": ref.CreatedAt.Format(time.RFC3339),
	}
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRefundNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrPaymentNotCaptured),
		errors.Is(err, ErrRefundAmountExceedsRefundable),
		errors.Is(err, ErrZeroRefundAmount):
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: err.Error()})
	case errors.Is(err, ErrInvalidTransition):
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.APIError{Code: "INVALID_STATE", Description: err.Error()})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error"})
	}
}
