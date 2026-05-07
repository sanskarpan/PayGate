package dispute

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/merchant"
)

// Handler exposes the dispute HTTP endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutesWithAuth wires dispute endpoints into mux under auth middleware.
func (h *Handler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(scope merchant.APIKeyScope, next http.Handler) http.Handler) {
	mux.Handle("POST /v1/disputes", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.create)))
	mux.Handle("GET /v1/disputes", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.list)))
	mux.Handle("GET /v1/disputes/{disputeID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.get)))
	mux.Handle("POST /v1/disputes/{disputeID}/submit-evidence", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.submitEvidence)))
	mux.Handle("POST /v1/disputes/{disputeID}/review", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.review)))
	mux.Handle("POST /v1/disputes/{disputeID}/resolve", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.resolve)))
	mux.Handle("POST /v1/disputes/{disputeID}/accept", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.accept)))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}

	var body struct {
		PaymentID    string  `json:"payment_id"`
		Reason       string  `json:"reason"`
		Amount       int64   `json:"amount"`
		Currency     string  `json:"currency"`
		DueBy        *int64  `json:"due_by"`
		SettlementID string  `json:"settlement_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}

	currency := body.Currency
	if currency == "" {
		currency = "INR"
	}

	var dueBy *time.Time
	if body.DueBy != nil {
		t := time.Unix(*body.DueBy, 0).UTC()
		dueBy = &t
	}

	d, err := h.svc.Create(r.Context(), p.MerchantID, body.PaymentID, body.Reason, body.Amount, currency, dueBy)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, present(d))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}

	limit := 25
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}

	disputes, err := h.svc.List(r.Context(), p.MerchantID, limit)
	if err != nil {
		handleError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(disputes))
	for _, d := range disputes {
		items = append(items, present(d))
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
	d, err := h.svc.Get(r.Context(), p.MerchantID, r.PathValue("disputeID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(d))
}

func (h *Handler) submitEvidence(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}

	var evidence map[string]any
	if err := json.NewDecoder(r.Body).Decode(&evidence); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}

	d, err := h.svc.SubmitEvidence(r.Context(), p.MerchantID, r.PathValue("disputeID"), evidence)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(d))
}

func (h *Handler) review(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	d, err := h.svc.StartReview(r.Context(), p.MerchantID, r.PathValue("disputeID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(d))
}

func (h *Handler) resolve(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}

	var body struct {
		Outcome string `json:"outcome"`
		Notes   string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}

	d, err := h.svc.Resolve(r.Context(), p.MerchantID, r.PathValue("disputeID"), DisputeState(body.Outcome), body.Notes)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(d))
}

func (h *Handler) accept(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	d, err := h.svc.Accept(r.Context(), p.MerchantID, r.PathValue("disputeID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(d))
}

func present(d Dispute) map[string]any {
	var evidenceSubmittedAt, dueBy, resolvedAt *int64
	if d.EvidenceSubmittedAt != nil {
		ts := d.EvidenceSubmittedAt.Unix()
		evidenceSubmittedAt = &ts
	}
	if d.DueBy != nil {
		ts := d.DueBy.Unix()
		dueBy = &ts
	}
	if d.ResolvedAt != nil {
		ts := d.ResolvedAt.Unix()
		resolvedAt = &ts
	}
	return map[string]any{
		"entity":               "dispute",
		"id":                   d.ID,
		"merchant_id":          d.MerchantID,
		"payment_id":           d.PaymentID,
		"settlement_id":        d.SettlementID,
		"status":               d.Status,
		"reason":               d.Reason,
		"amount":               d.Amount,
		"currency":             d.Currency,
		"evidence":             d.Evidence,
		"evidence_submitted_at": evidenceSubmittedAt,
		"due_by":               dueBy,
		"resolved_at":          resolvedAt,
		"notes":                d.Notes,
		"created_at":           d.CreatedAt.Unix(),
	}
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrDisputeNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrInvalidTransition):
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.APIError{Code: "INVALID_STATE", Description: err.Error()})
	case errors.Is(err, ErrDisputeTerminal):
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.APIError{Code: "DISPUTE_TERMINAL", Description: err.Error()})
	case errors.Is(err, ErrEvidenceAlreadySubmitted):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "EVIDENCE_ALREADY_SUBMITTED", Description: err.Error()})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error"})
	}
}
