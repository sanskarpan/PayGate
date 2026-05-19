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
	mux.Handle("POST /v1/settlements/batch", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.runBatch)))
	mux.Handle("POST /v1/settlements/partial", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.runPartialBatch)))
	mux.Handle("POST /v1/settlements/{settlementID}/hold", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.hold)))
	mux.Handle("POST /v1/settlements/{settlementID}/release", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.release)))
	mux.Handle("POST /v1/settlements/{settlementID}/rollback-marker", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.markRollback)))
}

func (h *Handler) runBatch(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}

	var body struct {
		PeriodStart *int64 `json:"period_start"`
		PeriodEnd   *int64 `json:"period_end"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
			return
		}
	}

	periodStart := time.Unix(0, 0)
	periodEnd := time.Now().UTC()
	if body.PeriodStart != nil {
		periodStart = time.Unix(*body.PeriodStart, 0)
	}
	if body.PeriodEnd != nil {
		periodEnd = time.Unix(*body.PeriodEnd, 0)
	}

	sttl, err := h.svc.RunBatch(r.Context(), p.MerchantID, periodStart, periodEnd)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, present(sttl))
}

func (h *Handler) runPartialBatch(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}

	var body struct {
		PaymentIDs []string `json:"payment_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.PaymentIDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "payment_ids is required and must be non-empty"})
		return
	}

	sttl, err := h.svc.RunPartialBatch(r.Context(), p.MerchantID, body.PaymentIDs)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, present(sttl))
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

func (h *Handler) markRollback(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
			return
		}
	}
	sttl, err := h.svc.MarkRollback(r.Context(), p.MerchantID, r.PathValue("settlementID"), body.Reason)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(sttl))
}

func present(s Settlement) map[string]any {
	var processedAt *int64
	if s.ProcessedAt != nil {
		ts := s.ProcessedAt.Unix()
		processedAt = &ts
	}
	return map[string]any{
		"id":                 s.ID,
		"entity":             "settlement",
		"merchant_id":        s.MerchantID,
		"status":             s.Status,
		"period_start":       s.PeriodStart.Unix(),
		"period_end":         s.PeriodEnd.Unix(),
		"total_amount":       s.TotalAmount,
		"total_fees":         s.TotalFees,
		"total_refunds":      s.TotalRefunds,
		"net_amount":         s.NetAmount,
		"payment_count":      s.PaymentCount,
		"currency":           s.Currency,
		"processed_at":       processedAt,
		"on_hold":            s.OnHold,
		"hold_reason":        s.HoldReason,
		"rollback_marked_at": presentTime(s.RollbackMarkedAt),
		"rollback_reason":    s.RollbackReason,
		"created_at":         s.CreatedAt.Unix(),
		"created_at_rfc":     s.CreatedAt.Format(time.RFC3339),
	}
}

func presentTime(ts *time.Time) any {
	if ts == nil {
		return nil
	}
	return ts.Unix()
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
	case errors.Is(err, ErrSettlementRollbackMarked):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "SETTLEMENT_ROLLBACK_MARKED", Description: err.Error()})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error"})
	}
}
