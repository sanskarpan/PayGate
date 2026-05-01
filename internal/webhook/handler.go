package webhook

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/merchant"
)

// Handler exposes the webhook HTTP endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutesWithAuth wires webhook endpoints into mux under auth.
func (h *Handler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(scope merchant.APIKeyScope, next http.Handler) http.Handler) {
	mux.Handle("POST /v1/webhooks", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.create)))
	mux.Handle("GET /v1/webhooks", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.list)))
	mux.Handle("GET /v1/webhooks/{webhookID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.get)))
	mux.Handle("PATCH /v1/webhooks/{webhookID}", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.update)))
	mux.Handle("DELETE /v1/webhooks/{webhookID}", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.delete)))
	mux.Handle("POST /v1/webhooks/{webhookID}/enable", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.enable)))
	mux.Handle("POST /v1/webhooks/{webhookID}/disable", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.disable)))
	mux.Handle("GET /v1/webhooks/{webhookID}/deliveries", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listDeliveries)))
	mux.Handle("POST /v1/webhooks/{webhookID}/rotate-secret", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.rotateSecret)))
	mux.Handle("POST /v1/webhooks/events/{eventID}/replay", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.replay)))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	sub, err := h.svc.CreateSubscription(r.Context(), CreateInput{
		MerchantID: p.MerchantID,
		URL:        req.URL,
		Events:     req.Events,
	})
	if err != nil {
		handleError(w, err)
		return
	}
	// Return secret only on creation — never again.
	httpx.WriteJSON(w, http.StatusCreated, presentWithSecret(sub))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	subs, err := h.svc.ListSubscriptions(r.Context(), p.MerchantID)
	if err != nil {
		handleError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(subs))
	for _, sub := range subs {
		items = append(items, present(sub))
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
	sub, err := h.svc.GetSubscription(r.Context(), p.MerchantID, r.PathValue("webhookID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(sub))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	sub, err := h.svc.UpdateSubscription(r.Context(), p.MerchantID, r.PathValue("webhookID"), UpdateInput{
		URL:    req.URL,
		Events: req.Events,
	})
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(sub))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	if err := h.svc.DeleteSubscription(r.Context(), p.MerchantID, r.PathValue("webhookID")); err != nil {
		handleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) enable(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	sub, err := h.svc.EnableSubscription(r.Context(), p.MerchantID, r.PathValue("webhookID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(sub))
}

func (h *Handler) disable(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	sub, err := h.svc.DisableSubscription(r.Context(), p.MerchantID, r.PathValue("webhookID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(sub))
}

func (h *Handler) listDeliveries(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	attempts, err := h.svc.ListDeliveryAttempts(r.Context(), p.MerchantID, r.PathValue("webhookID"))
	if err != nil {
		handleError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(attempts))
	for _, a := range attempts {
		items = append(items, presentAttempt(a))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity": "collection",
		"count":  len(items),
		"items":  items,
	})
}

func (h *Handler) rotateSecret(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	sub, err := h.svc.RotateSecret(r.Context(), p.MerchantID, r.PathValue("webhookID"))
	if err != nil {
		handleError(w, err)
		return
	}
	// Return new secret on rotation.
	httpx.WriteJSON(w, http.StatusOK, presentWithSecret(sub))
}

func (h *Handler) replay(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	n, err := h.svc.ReplayEvent(r.Context(), p.MerchantID, r.PathValue("eventID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity":     "replay",
		"event_id":   r.PathValue("eventID"),
		"deliveries": n,
	})
}

func present(sub WebhookSubscription) map[string]any {
	return map[string]any{
		"id":          sub.ID,
		"entity":      "webhook",
		"merchant_id": sub.MerchantID,
		"url":         sub.URL,
		"events":      sub.Events,
		"status":      sub.Status,
		"created_at":  sub.CreatedAt.Unix(),
		"updated_at":  sub.UpdatedAt.Unix(),
	}
}

func presentWithSecret(sub WebhookSubscription) map[string]any {
	m := present(sub)
	m["secret"] = sub.Secret
	return m
}

func presentAttempt(a WebhookDeliveryAttempt) map[string]any {
	var nextRetryAt *int64
	if a.NextRetryAt != nil {
		ts := a.NextRetryAt.Unix()
		nextRetryAt = &ts
	}
	return map[string]any{
		"id":              a.ID,
		"entity":          "webhook_delivery",
		"event_id":        a.EventID,
		"subscription_id": a.SubscriptionID,
		"status":          a.Status,
		"request_url":     a.RequestURL,
		"response_code":   a.ResponseCode,
		"response_body":   a.ResponseBody,
		"error":           a.ErrorMessage,
		"attempt_number":  a.AttemptNumber,
		"next_retry_at":   nextRetryAt,
		"created_at":      a.CreatedAt.Unix(),
		"created_at_rfc":  a.CreatedAt.Format(time.RFC3339),
	}
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSubscriptionNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrDeliveryAttemptNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrInvalidTransition):
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.APIError{Code: "INVALID_STATE", Description: err.Error()})
	case errors.Is(err, ErrInvalidURL), errors.Is(err, ErrNoEvents):
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: err.Error()})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error"})
	}
}
