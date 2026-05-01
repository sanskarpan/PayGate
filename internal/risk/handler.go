package risk

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
)

// Handler exposes risk event endpoints.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutesWithAuth wires risk endpoints behind authentication.
func (h *Handler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(scope string, next http.Handler) http.Handler) {
	mux.Handle("GET /v1/risk/events", wrap("read", http.HandlerFunc(h.listRiskEvents)))
	mux.Handle("GET /v1/risk/events/{id}", wrap("read", http.HandlerFunc(h.getRiskEvent)))
	mux.Handle("POST /v1/risk/events/{id}/resolve", wrap("admin", http.HandlerFunc(h.resolveRiskEvent)))
}

func (h *Handler) listRiskEvents(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("count"))
	unresolvedOnly := q.Get("unresolved") == "true"

	events, err := h.svc.ListRiskEvents(r.Context(), p.MerchantID, limit, unresolvedOnly)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{
			Code: "SERVER_ERROR", Description: "failed to list risk events",
		})
		return
	}

	items := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		items = append(items, riskEventToMap(ev))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity": "collection",
		"count":  len(items),
		"items":  items,
	})
}

func (h *Handler) getRiskEvent(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	ev, err := h.svc.GetRiskEvent(r.Context(), p.MerchantID, r.PathValue("id"))
	if err != nil {
		if errors.Is(err, ErrRiskEventNotFound) {
			httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: "risk event not found"})
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to get risk event"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, riskEventToMap(ev))
}

func (h *Handler) resolveRiskEvent(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}

	var body struct {
		ResolvedBy string `json:"resolved_by"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	resolvedBy := body.ResolvedBy
	if resolvedBy == "" {
		resolvedBy = p.UserID
	}

	if err := h.svc.ResolveRiskEvent(r.Context(), p.MerchantID, r.PathValue("id"), resolvedBy); err != nil {
		if errors.Is(err, ErrRiskEventNotFound) {
			httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: "risk event not found"})
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to resolve risk event"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

func riskEventToMap(ev RiskEvent) map[string]any {
	m := map[string]any{
		"id":              ev.ID,
		"payment_id":      ev.PaymentID,
		"score":           ev.Score,
		"action":          ev.Action,
		"triggered_rules": ev.TriggeredRules,
		"resolved":        ev.Resolved,
		"created_at":      ev.CreatedAt.Unix(),
	}
	if ev.ResolvedBy != "" {
		m["resolved_by"] = ev.ResolvedBy
	}
	if ev.ResolvedAt != nil {
		m["resolved_at"] = ev.ResolvedAt.Unix()
	}
	return m
}
