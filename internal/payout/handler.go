package payout

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/merchant"
	"github.com/sanskarpan/PayGate/internal/settlement"
)

// Handler exposes the payout HTTP endpoints.
type Handler struct {
	svc        *Service
	settlement *settlement.Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, settlementSvc *settlement.Service) *Handler {
	return &Handler{svc: svc, settlement: settlementSvc}
}

// RegisterRoutesWithAuth wires payout endpoints into mux under auth.
func (h *Handler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(scope merchant.APIKeyScope, next http.Handler) http.Handler) {
	mux.Handle("POST /v1/settlements/{settlementID}/payout", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.initiate)))
	mux.Handle("GET /v1/payouts", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.list)))
	mux.Handle("GET /v1/payouts/{payoutID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.get)))
}

func (h *Handler) initiate(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	settlementID := r.PathValue("settlementID")

	// Look up the settlement to get the net amount and currency.
	sttl, err := h.settlement.Get(r.Context(), p.MerchantID, settlementID)
	if err != nil {
		handleError(w, err)
		return
	}

	pout, err := h.svc.InitiatePayoutForSettlement(r.Context(), p.MerchantID, settlementID, sttl.NetAmount, sttl.Currency)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, present(pout))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	payouts, err := h.svc.List(r.Context(), p.MerchantID, limit)
	if err != nil {
		handleError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(payouts))
	for _, po := range payouts {
		items = append(items, present(po))
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
	pout, err := h.svc.Get(r.Context(), p.MerchantID, r.PathValue("payoutID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(pout))
}

func present(p Payout) map[string]any {
	var initiatedAt, completedAt any
	if p.InitiatedAt != nil {
		initiatedAt = p.InitiatedAt.Unix()
	}
	if p.CompletedAt != nil {
		completedAt = p.CompletedAt.Unix()
	}
	return map[string]any{
		"entity":         "payout",
		"id":             p.ID,
		"merchant_id":    p.MerchantID,
		"settlement_id":  p.SettlementID,
		"status":         p.Status,
		"amount":         p.Amount,
		"currency":       p.Currency,
		"bank_reference": p.BankReference,
		"failure_reason": p.FailureReason,
		"initiated_at":   initiatedAt,
		"completed_at":   completedAt,
		"created_at":     p.CreatedAt.Unix(),
	}
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrPayoutNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrPayoutAlreadyExists):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "PAYOUT_ALREADY_EXISTS", Description: err.Error()})
	case errors.Is(err, ErrInvalidTransition):
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.APIError{Code: "INVALID_STATE", Description: err.Error()})
	case errors.Is(err, ErrSettlementNotProcessed):
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.APIError{Code: "SETTLEMENT_NOT_PROCESSED", Description: err.Error()})
	default:
		// Check for settlement not found too.
		var errBody struct{ Code string }
		if b, merr := json.Marshal(err); merr == nil {
			_ = json.Unmarshal(b, &errBody)
		}
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error"})
	}
}
