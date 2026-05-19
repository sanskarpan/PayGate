package saga

import (
	"encoding/json"
	"errors"
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
	mux.Handle("GET /v1/sagas", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.list)))
	mux.Handle("GET /v1/sagas/{sagaID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.get)))
	mux.Handle("GET /v1/sagas/{sagaID}/dead-letters", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listDeadLetters)))
	mux.Handle("GET /v1/sagas/{sagaID}/dispatches", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listDispatches)))
	mux.Handle("GET /v1/sagas/{sagaID}/actions", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listActions)))
	mux.Handle("POST /v1/sagas/{sagaID}/replay", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.replay)))
	mux.Handle("POST /v1/sagas/{sagaID}/override", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.override)))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.List(r.Context(), p.MerchantID, limit)
	if err != nil {
		handleError(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, present(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity": "collection",
		"count":  len(payload),
		"items":  payload,
	})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	item, err := h.svc.Get(r.Context(), p.MerchantID, r.PathValue("sagaID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(item))
}

func (h *Handler) replay(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		DryRun bool   `json:"dry_run"`
		Force  bool   `json:"force"`
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	item, err := h.svc.Replay(r.Context(), ReplayInput{
		MerchantID: p.MerchantID,
		SagaID:     r.PathValue("sagaID"),
		Force:      req.Force,
		DryRun:     req.DryRun,
		ActorType:  principalActorType(p),
		ActorID:    principalActorID(p),
		Reason:     req.Reason,
	})
	if err != nil {
		handleError(w, err)
		return
	}
	statusCode := http.StatusAccepted
	if req.DryRun {
		statusCode = http.StatusOK
	}
	httpx.WriteJSON(w, statusCode, present(item))
}

func (h *Handler) override(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	item, err := h.svc.Override(r.Context(), OverrideInput{
		MerchantID: p.MerchantID,
		SagaID:     r.PathValue("sagaID"),
		Action:     OverrideAction(req.Action),
		ActorType:  principalActorType(p),
		ActorID:    principalActorID(p),
		Reason:     req.Reason,
	})
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, present(item))
}

func (h *Handler) listDeadLetters(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListDeadLetters(r.Context(), p.MerchantID, r.PathValue("sagaID"), limit)
	if err != nil {
		handleError(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentDeadLetter(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) listActions(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListOperatorActions(r.Context(), p.MerchantID, r.PathValue("sagaID"), limit)
	if err != nil {
		handleError(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentAction(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) listDispatches(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListDispatches(r.Context(), p.MerchantID, r.PathValue("sagaID"), limit)
	if err != nil {
		handleError(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentDispatch(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func present(inst Instance) map[string]any {
	steps := make([]map[string]any, 0, len(inst.Steps))
	for _, step := range inst.Steps {
		var leasedAt any
		var completedAt any
		if step.LeasedAt != nil {
			leasedAt = step.LeasedAt.Unix()
		}
		if step.CompletedAt != nil {
			completedAt = step.CompletedAt.Unix()
		}
		steps = append(steps, map[string]any{
			"id":             step.ID,
			"saga_id":        step.SagaID,
			"step_index":     step.StepIndex,
			"step_name":      step.StepName,
			"step_kind":      step.StepKind,
			"status":         step.Status,
			"command_name":   step.CommandName,
			"command_id":     step.CommandID,
			"input_payload":  step.InputPayload,
			"output_payload": step.OutputPayload,
			"error_code":     step.ErrorCode,
			"error_message":  step.ErrorMessage,
			"attempt_count":  step.AttemptCount,
			"max_attempts":   step.MaxAttempts,
			"leased_by":      step.LeasedBy,
			"leased_at":      leasedAt,
			"completed_at":   completedAt,
			"created_at":     step.CreatedAt.Unix(),
		})
	}
	var lastLeasedAt any
	var deadlineAt any
	var timeoutAt any
	var completedAt any
	if inst.LastLeasedAt != nil {
		lastLeasedAt = inst.LastLeasedAt.Unix()
	}
	if inst.DeadlineAt != nil {
		deadlineAt = inst.DeadlineAt.Unix()
	}
	if inst.TimeoutAt != nil {
		timeoutAt = inst.TimeoutAt.Unix()
	}
	if inst.CompletedAt != nil {
		completedAt = inst.CompletedAt.Unix()
	}
	return map[string]any{
		"entity":             "saga",
		"id":                 inst.ID,
		"merchant_id":        inst.MerchantID,
		"saga_type":          inst.SagaType,
		"status":             inst.Status,
		"correlation_id":     inst.CorrelationID,
		"causation_id":       inst.CausationID,
		"input_payload":      inst.InputPayload,
		"context_payload":    inst.ContextPayload,
		"current_step_index": inst.CurrentStepIndex,
		"failure_code":       inst.FailureCode,
		"failure_reason":     inst.FailureReason,
		"leased_by":          inst.LeasedBy,
		"last_leased_at":     lastLeasedAt,
		"replay_count":       inst.ReplayCount,
		"deadline_at":        deadlineAt,
		"timeout_at":         timeoutAt,
		"started_at":         inst.StartedAt.Unix(),
		"completed_at":       completedAt,
		"created_at":         inst.CreatedAt.Unix(),
		"steps":              steps,
	}
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSagaNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrSagaNotReplayable):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "SAGA_NOT_REPLAYABLE", Description: err.Error()})
	case errors.Is(err, ErrInvalidOverride):
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "INVALID_OVERRIDE", Description: err.Error()})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error"})
	}
}

func presentDeadLetter(item DeadLetter) map[string]any {
	return map[string]any{
		"entity":           "saga_dead_letter",
		"id":               item.ID,
		"saga_id":          item.SagaID,
		"step_id":          item.StepID,
		"merchant_id":      item.MerchantID,
		"dead_letter_type": item.DeadLetterType,
		"command_name":     item.CommandName,
		"command_id":       item.CommandID,
		"error_code":       item.ErrorCode,
		"error_message":    item.ErrorMessage,
		"payload":          item.Payload,
		"created_at":       item.CreatedAt.Unix(),
	}
}

func presentAction(item OperatorAction) map[string]any {
	return map[string]any{
		"entity":      "saga_operator_action",
		"id":          item.ID,
		"saga_id":     item.SagaID,
		"merchant_id": item.MerchantID,
		"action":      item.Action,
		"actor_type":  item.ActorType,
		"actor_id":    item.ActorID,
		"reason":      item.Reason,
		"payload":     item.Payload,
		"created_at":  item.CreatedAt.Unix(),
	}
}

func presentDispatch(item CommandDispatch) map[string]any {
	var ackedAt any
	var nackedAt any
	if item.AckedAt != nil {
		ackedAt = item.AckedAt.Unix()
	}
	if item.NackedAt != nil {
		nackedAt = item.NackedAt.Unix()
	}
	return map[string]any{
		"entity":                "saga_command_dispatch",
		"id":                    item.ID,
		"saga_id":               item.SagaID,
		"step_id":               item.StepID,
		"merchant_id":           item.MerchantID,
		"command_name":          item.CommandName,
		"command_id":            item.CommandID,
		"dispatch_attempt":      item.DispatchAttempt,
		"status":                item.Status,
		"leased_by":             item.LeasedBy,
		"leased_at":             item.LeasedAt.Unix(),
		"acked_at":              ackedAt,
		"nacked_at":             nackedAt,
		"retry_backoff_seconds": item.RetryBackoffSeconds,
		"error_code":            item.ErrorCode,
		"error_message":         item.ErrorMessage,
		"error_classification":  item.ErrorClassification,
		"input_payload":         item.InputPayload,
		"output_payload":        item.OutputPayload,
		"created_at":            item.CreatedAt.Unix(),
	}
}

func principalActorType(p httpx.Principal) string {
	if p.UserID != "" {
		return "dashboard_user"
	}
	return "api_key"
}

func principalActorID(p httpx.Principal) string {
	if p.UserID != "" {
		return p.UserID
	}
	return p.KeyID
}
