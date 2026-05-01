package settlement

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/merchant"
)

// Handler exposes the settlement HTTP endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutesWithAuth wires settlement endpoints into mux under auth.
func (h *Handler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(scope merchant.APIKeyScope, next http.Handler) http.Handler) {
	mux.Handle("GET /v1/settlements", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.list)))
	mux.Handle("GET /v1/settlements/{settlementID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.get)))
	mux.Handle("POST /v1/settlements/{settlementID}/hold", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.hold)))
	mux.Handle("POST /v1/settlements/{settlementID}/release", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.release)))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	settlements, err := h.svc.List(r.Context(), p.MerchantID)
	if err != nil {
		handleError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(settlements))
	for _, s := range settlements {
		items = append(items, present(s))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity": "collection",
		"count":  len(items),
		"items":  items,
	})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	sttl, lineItems, err := h.svc.GetItems(r.Context(), p.MerchantID, r.PathValue("settlementID"))
	if err != nil {
		handleError(w, err)
		return
	}
	resp := present(sttl)
	itemsJSON := make([]map[string]any, 0, len(lineItems))
	for _, item := range lineItems {
		itemsJSON = append(itemsJSON, presentItem(item))
	}
	resp["items"] = itemsJSON
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) hold(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	settlementID := r.PathValue("settlementID")

	var body struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
			return
		}
	}

	if err := h.svc.Hold(r.Context(), p.MerchantID, settlementID, body.Reason); err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":      settlementID,
		"on_hold": true,
	})
}

func (h *Handler) release(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	settlementID := r.PathValue("settlementID")

	if err := h.svc.Release(r.Context(), p.MerchantID, settlementID); err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":      settlementID,
		"on_hold": false,
	})
}

func present(s Settlement) map[string]any {
	var processedAt *int64
	if s.ProcessedAt != nil {
		ts := s.ProcessedAt.Unix()
		processedAt = &ts
	}
	return map[string]any{
		"id":            s.ID,
		"entity":        "settlement",
		"merchant_id":   s.MerchantID,
		"status":        s.Status,
		"period_start":  s.PeriodStart.Unix(),
		"period_end":    s.PeriodEnd.Unix(),
		"total_amount":  s.TotalAmount,
		"total_fees":    s.TotalFees,
		"total_refunds": s.TotalRefunds,
		"net_amount":    s.NetAmount,
		"payment_count": s.PaymentCount,
		"currency":      s.Currency,
		"processed_at":  processedAt,
		"created_at":    s.CreatedAt.Unix(),
		"created_at_rfc": s.CreatedAt.Format(time.RFC3339),
	}
}

func presentItem(item SettlementItem) map[string]any {
	return map[string]any{
		"id":            item.ID,
		"entity":        "settlement_item",
		"settlement_id": item.SettlementID,
		"payment_id":    item.PaymentID,
		"amount":        item.Amount,
		"fee":           item.Fee,
		"refunds":       item.Refunds,
		"net":           item.Net,
		"currency":      item.Currency,
		"created_at":    item.CreatedAt.Unix(),
	}
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSettlementNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrNoEligiblePayments):
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.APIError{Code: "NO_ELIGIBLE_PAYMENTS", Description: err.Error()})
	case errors.Is(err, ErrInvalidTransition):
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.APIError{Code: "INVALID_STATE", Description: err.Error()})
	case errors.Is(err, ErrSettlementOnHold):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "SETTLEMENT_ON_HOLD", Description: err.Error()})
	case errors.Is(err, ErrSettlementNotOnHold):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "SETTLEMENT_NOT_ON_HOLD", Description: err.Error()})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error"})
	}
}
