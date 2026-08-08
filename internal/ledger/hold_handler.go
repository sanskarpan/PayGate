package ledger

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/merchant"
)

type HoldHandler struct {
	svc *Service
}

func NewHoldHandler(svc *Service) *HoldHandler {
	return &HoldHandler{svc: svc}
}

func (h *HoldHandler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(scope merchant.APIKeyScope, next http.Handler) http.Handler) {
	mux.Handle("POST /v1/ledger/holds", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.create)))
	mux.Handle("GET /v1/ledger/holds", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.list)))
	mux.Handle("GET /v1/ledger/holds/{holdID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.get)))
	mux.Handle("POST /v1/ledger/holds/{holdID}/extend", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.extend)))
	mux.Handle("POST /v1/ledger/holds/{holdID}/release", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.release)))
	mux.Handle("POST /v1/ledger/holds/{holdID}/commit", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.commit)))
	mux.Handle("POST /v1/ledger/holds/{holdID}/expire", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.expire)))
}

func (h *HoldHandler) create(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		AccountCode string `json:"account_code"`
		SourceType  string `json:"source_type"`
		SourceID    string `json:"source_id"`
		Reason      string `json:"reason"`
		Currency    string `json:"currency"`
		Amount      int64  `json:"amount"`
		ExpiresAt   *int64 `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		tm := time.Unix(*req.ExpiresAt, 0).UTC()
		expiresAt = &tm
	}
	out, err := h.svc.CreateHold(r.Context(), CreateHoldInput{
		MerchantID:     p.MerchantID,
		AccountCode:    req.AccountCode,
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		Reason:         req.Reason,
		Currency:       req.Currency,
		Amount:         req.Amount,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		handleHoldError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentHold(out))
}

func (h *HoldHandler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListHolds(r.Context(), p.MerchantID, limit)
	if err != nil {
		handleHoldError(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentHold(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity": "collection",
		"count":  len(payload),
		"items":  payload,
	})
}

func (h *HoldHandler) get(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	item, err := h.svc.GetHold(r.Context(), p.MerchantID, r.PathValue("holdID"))
	if err != nil {
		handleHoldError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentHold(item))
}

func (h *HoldHandler) release(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	item, err := h.svc.ReleaseHold(r.Context(), ReleaseHoldInput{
		MerchantID: p.MerchantID,
		HoldID:     r.PathValue("holdID"),
	})
	if err != nil {
		handleHoldError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentHold(item))
}

func (h *HoldHandler) extend(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		ExpiresAt *int64 `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		tm := time.Unix(*req.ExpiresAt, 0).UTC()
		expiresAt = &tm
	}
	item, err := h.svc.ExtendHold(r.Context(), ExtendHoldInput{
		MerchantID: p.MerchantID,
		HoldID:     r.PathValue("holdID"),
		ExpiresAt:  expiresAt,
	})
	if err != nil {
		handleHoldError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentHold(item))
}

func (h *HoldHandler) commit(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		TargetAccountCode string `json:"target_account_code"`
		Description       string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	item, err := h.svc.CommitHold(r.Context(), CommitHoldInput{
		MerchantID:        p.MerchantID,
		HoldID:            r.PathValue("holdID"),
		TargetAccountCode: req.TargetAccountCode,
		Description:       req.Description,
	})
	if err != nil {
		handleHoldError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentHold(item))
}

func (h *HoldHandler) expire(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	item, err := h.svc.ExpireHold(r.Context(), ExpireHoldInput{
		MerchantID: p.MerchantID,
		HoldID:     r.PathValue("holdID"),
	})
	if err != nil {
		handleHoldError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentHold(item))
}

func presentHold(h Hold) map[string]any {
	var expiresAt any
	var releasedAt any
	var committedAt any
	if h.ExpiresAt != nil {
		expiresAt = h.ExpiresAt.Unix()
	}
	if h.ReleasedAt != nil {
		releasedAt = h.ReleasedAt.Unix()
	}
	if h.CommittedAt != nil {
		committedAt = h.CommittedAt.Unix()
	}
	return map[string]any{
		"entity":              "ledger_hold",
		"id":                  h.ID,
		"merchant_id":         h.MerchantID,
		"account_code":        h.AccountCode,
		"source_type":         h.SourceType,
		"source_id":           h.SourceID,
		"reason":              h.Reason,
		"currency":            h.Currency,
		"amount":              h.Amount,
		"status":              h.Status,
		"idempotency_key":     h.IdempotencyKey,
		"target_account_code": h.TargetAccountCode,
		"description":         h.Description,
		"business_reference": map[string]any{
			"source_type": h.SourceType,
			"source_id":   h.SourceID,
		},
		"ledger_reference": map[string]any{
			"commit_source_type": "ledger_hold_commit",
			"commit_source_id":   h.ID,
		},
		"expires_at":   expiresAt,
		"released_at":  releasedAt,
		"committed_at": committedAt,
		"created_at":   h.CreatedAt.Unix(),
	}
}

func handleHoldError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidHoldInput):
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: err.Error()})
	case errors.Is(err, ErrHoldNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrHoldNotActive), errors.Is(err, ErrHoldAlreadyHandled):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "INVALID_STATE", Description: err.Error()})
	case errors.Is(err, ErrHoldInsufficient):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "INSUFFICIENT_FUNDS", Description: err.Error()})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error"})
	}
}
