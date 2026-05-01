package audit

import (
	"net/http"
	"strconv"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutesWithAuth wires the audit log API under authentication.
func (h *Handler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(next http.Handler) http.Handler) {
	mux.Handle("GET /v1/audit-logs", wrap(http.HandlerFunc(h.listAuditLogs)))
}

func (h *Handler) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("count"))

	logs, err := h.svc.List(r.Context(), ListInput{
		MerchantID:   p.MerchantID,
		ActorID:      q.Get("actor_id"),
		ResourceType: q.Get("resource_type"),
		ResourceID:   q.Get("resource_id"),
		Limit:        limit,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{
			Code:        "SERVER_ERROR",
			Description: "failed to list audit logs",
			Source:      "internal",
			Step:        "audit_log_list",
			Reason:      "unhandled_exception",
		})
		return
	}

	items := make([]map[string]any, 0, len(logs))
	for _, l := range logs {
		items = append(items, map[string]any{
			"id":             l.ID,
			"actor_id":       l.ActorID,
			"actor_email":    l.ActorEmail,
			"actor_type":     l.ActorType,
			"action":         l.Action,
			"resource_type":  l.ResourceType,
			"resource_id":    l.ResourceID,
			"changes":        l.Changes,
			"ip_address":     l.IPAddress,
			"correlation_id": l.CorrelationID,
			"created_at":     l.CreatedAt.Unix(),
		})
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity": "collection",
		"count":  len(items),
		"items":  items,
	})
}
