package tokenization

import (
	"encoding/json"
	"errors"
	"net/http"
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
	mux.Handle("POST /v1/card-tokens", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.createCardToken)))
	mux.Handle("GET /v1/card-tokens", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listCardTokens)))
	mux.Handle("GET /v1/card-tokens/{tokenID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getCardToken)))
	mux.Handle("POST /v1/card-tokens/{tokenID}/disable", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.disableCardToken)))
	mux.Handle("DELETE /v1/card-tokens/{tokenID}", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.deleteCardToken)))
}

func (h *Handler) createCardToken(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		CardNumber       string `json:"card_number"`
		ExpMonth         int    `json:"exp_month"`
		ExpYear          int    `json:"exp_year"`
		CardholderName   string `json:"cardholder_name"`
		CustomerRef      string `json:"customer_ref"`
		Reusable         bool   `json:"reusable"`
		NetworkReference string `json:"network_reference"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	out, err := h.svc.CreateCardToken(r.Context(), CreateCardTokenInput{
		MerchantID:       p.MerchantID,
		CardNumber:       req.CardNumber,
		ExpMonth:         req.ExpMonth,
		ExpYear:          req.ExpYear,
		CardholderName:   req.CardholderName,
		CustomerRef:      req.CustomerRef,
		Reusable:         req.Reusable,
		NetworkReference: req.NetworkReference,
	})
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, present(out))
}

func (h *Handler) getCardToken(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.GetCardToken(r.Context(), p.MerchantID, r.PathValue("tokenID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(out))
}

func (h *Handler) listCardTokens(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	customerRef := r.URL.Query().Get("customer_ref")
	items, err := h.svc.ListCardTokens(r.Context(), p.MerchantID, customerRef, 100)
	if err != nil {
		handleError(w, err)
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, present(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *Handler) disableCardToken(w http.ResponseWriter, r *http.Request) {
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
	out, err := h.svc.DisableCardToken(r.Context(), p.MerchantID, r.PathValue("tokenID"), req.Reason)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(out))
}

func (h *Handler) deleteCardToken(w http.ResponseWriter, r *http.Request) {
	h.disableCardToken(w, r)
}

func present(out CardToken) map[string]any {
	resp := map[string]any{
		"id":          out.ID,
		"entity":      "card_token",
		"status":      out.Status,
		"token_class": out.TokenClass,
		"brand":       out.Brand,
		"last4":       out.Last4,
		"bin":         out.BIN,
		"exp_month":   out.ExpMonth,
		"exp_year":    out.ExpYear,
		"created_at":  out.CreatedAt.Unix(),
	}
	if out.CustomerRef != "" {
		resp["customer_ref"] = out.CustomerRef
	}
	if out.NetworkReference != "" {
		resp["network_reference"] = out.NetworkReference
	}
	if out.IssuerName != "" {
		resp["issuer_name"] = out.IssuerName
	}
	if out.IssuerCountry != "" {
		resp["issuer_country"] = out.IssuerCountry
	}
	if out.CardCountry != "" {
		resp["card_country"] = out.CardCountry
	}
	if out.FundingType != "" {
		resp["funding_type"] = out.FundingType
	}
	if out.NetworkTokenType != "" {
		resp["network_token_type"] = out.NetworkTokenType
	}
	if out.LastUsedAt != nil {
		resp["last_used_at"] = out.LastUsedAt.Unix()
	}
	if out.ConsumedAt != nil {
		resp["consumed_at"] = out.ConsumedAt.Unix()
	}
	if out.DisabledAt != nil {
		resp["disabled_at"] = out.DisabledAt.Unix()
	}
	return resp
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidCardNumber), errors.Is(err, ErrInvalidExpiry):
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: err.Error()})
	case errors.Is(err, ErrCardTokenNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrCardTokenExists):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "CARD_TOKEN_EXISTS", Description: err.Error()})
	case errors.Is(err, ErrCardTokenInactive):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: err.Error()})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error", Metadata: map[string]any{"at": time.Now().UTC().Unix()}})
	}
}
