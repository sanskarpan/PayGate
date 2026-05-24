package retention

import (
	"encoding/json"
	"net/http"
	"strconv"

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
	mux.Handle("GET /v1/retention/policies", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listPolicies)))
	mux.Handle("PUT /v1/retention/policies/{artifactClass}", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.upsertPolicy)))
	mux.Handle("GET /v1/retention/holds", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listHolds)))
	mux.Handle("POST /v1/retention/holds", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.createHold)))
	mux.Handle("POST /v1/retention/holds/{holdID}/release", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.releaseHold)))
	mux.Handle("GET /v1/retention/runs", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listRuns)))
	mux.Handle("POST /v1/retention/run", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.runNow)))
}

func (h *Handler) listPolicies(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListPolicies(r.Context())
	if err != nil {
		writeRetentionError(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentPolicy(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) upsertPolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action     PolicyAction `json:"action"`
		RetainDays int          `json:"retain_days"`
		Enabled    bool         `json:"enabled"`
		UpdatedBy  string       `json:"updated_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
		return
	}
	item, err := h.svc.UpsertPolicy(r.Context(), Policy{
		ArtifactClass: ArtifactClass(r.PathValue("artifactClass")),
		Action:        req.Action,
		RetainDays:    req.RetainDays,
		Enabled:       req.Enabled,
		UpdatedBy:     req.UpdatedBy,
	})
	if err != nil {
		writeRetentionError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentPolicy(item))
}

func (h *Handler) listHolds(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListLegalHolds(r.Context(), limit)
	if err != nil {
		writeRetentionError(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentHold(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) createHold(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ArtifactClass ArtifactClass `json:"artifact_class"`
		MerchantID    string        `json:"merchant_id"`
		ArtifactID    string        `json:"artifact_id"`
		Reason        string        `json:"reason"`
		CreatedBy     string        `json:"created_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
		return
	}
	item, err := h.svc.CreateLegalHold(r.Context(), CreateLegalHoldInput(req))
	if err != nil {
		writeRetentionError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentHold(item))
}

func (h *Handler) releaseHold(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.ReleaseLegalHold(r.Context(), r.PathValue("holdID"))
	if err != nil {
		writeRetentionError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentHold(item))
}

func (h *Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListRuns(r.Context(), limit)
	if err != nil {
		writeRetentionError(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentRun(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) runNow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ArtifactClass ArtifactClass `json:"artifact_class"`
		ActorType     string        `json:"actor_type"`
		ActorID       string        `json:"actor_id"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.ArtifactClass == "" {
		items, err := h.svc.RunAll(r.Context(), req.ActorType, req.ActorID)
		if err != nil {
			writeRetentionError(w, err)
			return
		}
		payload := make([]map[string]any, 0, len(items))
		for _, item := range items {
			payload = append(payload, presentRun(item))
		}
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
		return
	}
	item, err := h.svc.RunPolicy(r.Context(), RunInput(req))
	if err != nil {
		writeRetentionError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, presentRun(item))
}

func presentPolicy(item Policy) map[string]any {
	return map[string]any{
		"entity":         "retention_policy",
		"artifact_class": item.ArtifactClass,
		"action":         item.Action,
		"retain_days":    item.RetainDays,
		"enabled":        item.Enabled,
		"updated_by":     item.UpdatedBy,
		"created_at":     item.CreatedAt.Unix(),
		"updated_at":     item.UpdatedAt.Unix(),
	}
}

func presentHold(item LegalHold) map[string]any {
	var releasedAt any
	if item.ReleasedAt != nil {
		releasedAt = item.ReleasedAt.Unix()
	}
	return map[string]any{
		"entity":         "legal_hold",
		"id":             item.ID,
		"artifact_class": item.ArtifactClass,
		"merchant_id":    item.MerchantID,
		"artifact_id":    item.ArtifactID,
		"reason":         item.Reason,
		"created_by":     item.CreatedBy,
		"created_at":     item.CreatedAt.Unix(),
		"released_at":    releasedAt,
	}
}

func presentRun(item Run) map[string]any {
	var completedAt any
	if item.CompletedAt != nil {
		completedAt = item.CompletedAt.Unix()
	}
	return map[string]any{
		"entity":         "retention_run",
		"id":             item.ID,
		"artifact_class": item.ArtifactClass,
		"action":         item.Action,
		"status":         item.Status,
		"affected_count": item.AffectedCount,
		"error_message":  item.ErrorMessage,
		"actor_type":     item.ActorType,
		"actor_id":       item.ActorID,
		"started_at":     item.StartedAt.Unix(),
		"completed_at":   completedAt,
	}
}

func writeRetentionError(w http.ResponseWriter, err error) {
	httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error"})
}
