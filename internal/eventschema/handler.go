package eventschema

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
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
	mux.Handle("GET /v1/event-schemas", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listSchemas)))
	mux.Handle("POST /v1/event-schemas", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.createSchema)))
	mux.Handle("GET /v1/event-schemas/{subject}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getSchema)))
	mux.Handle("POST /v1/event-schemas/{subject}/versions", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.createVersion)))
	mux.Handle("GET /v1/event-schemas/{subject}/versions/{version}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getVersion)))
	mux.Handle("POST /v1/event-schemas/{subject}/versions/{version}/activate", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.activateVersion)))
	mux.Handle("GET /v1/event-schemas/{subject}/compare", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.compareVersions)))
	mux.Handle("POST /v1/event-schemas/{subject}/rollouts", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.createRollout)))
	mux.Handle("GET /v1/event-schema-rollouts/{rolloutID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getRollout)))
	mux.Handle("POST /v1/event-schema-rollouts/{rolloutID}/ack", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.ackRollout)))
}

func (h *Handler) listSchemas(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListSchemas(r.Context())
	if err != nil {
		handleError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, presentSchema(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(out), "items": out})
}

func (h *Handler) createSchema(w http.ResponseWriter, r *http.Request) {
	var req CreateSchemaInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid schema request body"})
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	req.EventType = strings.TrimSpace(req.EventType)
	req.TopicName = strings.TrimSpace(req.TopicName)
	req.Owner = strings.TrimSpace(req.Owner)
	if req.Subject == "" || req.EventType == "" || req.TopicName == "" || req.Owner == "" {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "subject, event_type, topic_name, and owner are required"})
		return
	}
	item, err := h.svc.CreateSchema(r.Context(), req)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentSchema(item))
}

func (h *Handler) getSchema(w http.ResponseWriter, r *http.Request) {
	item, versions, checks, err := h.svc.GetSchema(r.Context(), r.PathValue("subject"))
	if err != nil {
		handleError(w, err)
		return
	}
	versionPayload := make([]map[string]any, 0, len(versions))
	for _, version := range versions {
		versionPayload = append(versionPayload, presentVersion(version))
	}
	checkPayload := make([]map[string]any, 0, len(checks))
	for _, check := range checks {
		checkPayload = append(checkPayload, presentCheck(check))
	}
	resp := presentSchema(item)
	resp["versions"] = versionPayload
	resp["checks"] = checkPayload
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) createVersion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version       string         `json:"version"`
		Schema        Document       `json:"schema"`
		SamplePayload map[string]any `json:"sample_payload"`
		ReviewLink    string         `json:"review_link"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid schema version request body"})
		return
	}
	version, checks, err := h.svc.RegisterVersion(r.Context(), CreateVersionInput{
		Subject:       r.PathValue("subject"),
		Version:       strings.TrimSpace(req.Version),
		Schema:        req.Schema,
		SamplePayload: req.SamplePayload,
		ReviewLink:    strings.TrimSpace(req.ReviewLink),
	})
	if err != nil {
		if errors.Is(err, ErrIncompatibleSchema) {
			payload := make([]map[string]any, 0, len(checks))
			for _, check := range checks {
				payload = append(payload, presentCheck(check))
			}
			httpx.WriteError(w, http.StatusConflict, httpx.APIError{
				Code:        "INCOMPATIBLE_SCHEMA",
				Description: err.Error(),
				Metadata:    map[string]any{"checks": payload},
			})
			return
		}
		handleError(w, err)
		return
	}
	resp := presentVersion(version)
	if len(checks) > 0 {
		payload := make([]map[string]any, 0, len(checks))
		for _, check := range checks {
			payload = append(payload, presentCheck(check))
		}
		resp["checks"] = payload
	}
	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) getVersion(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.GetVersion(r.Context(), r.PathValue("subject"), r.PathValue("version"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentVersion(item))
}

func (h *Handler) activateVersion(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.ActivateVersion(r.Context(), ActivateVersionInput{
		Subject: r.PathValue("subject"),
		Version: r.PathValue("version"),
	})
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, presentVersion(item))
}

func (h *Handler) compareVersions(w http.ResponseWriter, r *http.Request) {
	checks, err := h.svc.CompareVersions(r.Context(), CompareVersionsInput{
		Subject:     r.PathValue("subject"),
		FromVersion: strings.TrimSpace(r.URL.Query().Get("from")),
		ToVersion:   strings.TrimSpace(r.URL.Query().Get("to")),
	})
	if err != nil {
		handleError(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(checks))
	for _, check := range checks {
		payload = append(payload, presentCheck(check))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity":  "schema_comparison",
		"subject": r.PathValue("subject"),
		"checks":  payload,
	})
}

func (h *Handler) createRollout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FromVersion     string `json:"from_version"`
		ToVersion       string `json:"to_version"`
		CutoverDeadline any    `json:"cutover_deadline"`
		Notes           string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid schema rollout request body"})
		return
	}
	deadlineTime, err := parseCutoverDeadline(req.CutoverDeadline)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid cutover_deadline"})
		return
	}
	item, err := h.svc.CreateRollout(r.Context(), CreateRolloutInput{
		Subject:         r.PathValue("subject"),
		FromVersion:     strings.TrimSpace(req.FromVersion),
		ToVersion:       strings.TrimSpace(req.ToVersion),
		CutoverDeadline: deadlineTime,
		Notes:           strings.TrimSpace(req.Notes),
	})
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentRollout(item))
}

func parseCutoverDeadline(raw any) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}

	switch value := raw.(type) {
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, nil
		}
		t, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return nil, err
		}
		utc := t.UTC()
		return &utc, nil
	case float64:
		if value <= 0 {
			return nil, nil
		}
		t := time.Unix(int64(value), 0).UTC()
		return &t, nil
	default:
		return nil, errors.New("unsupported cutover deadline type")
	}
}

func (h *Handler) getRollout(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.GetRollout(r.Context(), r.PathValue("rolloutID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentRollout(item))
}

func (h *Handler) ackRollout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConsumerName        string `json:"consumer_name"`
		AcknowledgedVersion string `json:"acknowledged_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid rollout ack request body"})
		return
	}
	item, err := h.svc.AckRollout(r.Context(), AckRolloutInput{
		RolloutID:           r.PathValue("rolloutID"),
		ConsumerName:        strings.TrimSpace(req.ConsumerName),
		AcknowledgedVersion: strings.TrimSpace(req.AcknowledgedVersion),
	})
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentRolloutConsumer(item))
}

func presentSchema(item Schema) map[string]any {
	return map[string]any{
		"entity":      "event_schema",
		"id":          item.ID,
		"subject":     item.Subject,
		"event_type":  item.EventType,
		"topic_name":  item.TopicName,
		"owner":       item.Owner,
		"review_link": item.ReviewLink,
		"created_at":  item.CreatedAt.Unix(),
		"updated_at":  item.UpdatedAt.Unix(),
	}
}

func presentVersion(item Version) map[string]any {
	resp := map[string]any{
		"entity":                "event_schema_version",
		"id":                    item.ID,
		"subject":               item.Subject,
		"version":               item.Version,
		"status":                item.Status,
		"schema":                item.Schema,
		"sample_payload":        item.SamplePayload,
		"review_link":           item.ReviewLink,
		"compatibility_summary": item.CompatibilitySummary,
		"compatibility_details": item.CompatibilityDetails,
		"created_at":            item.CreatedAt.Unix(),
		"updated_at":            item.UpdatedAt.Unix(),
	}
	if item.ActivatedAt != nil {
		resp["activated_at"] = item.ActivatedAt.Unix()
	}
	if item.DeprecatedAt != nil {
		resp["deprecated_at"] = item.DeprecatedAt.Unix()
	}
	return resp
}

func presentCheck(item CompatibilityCheck) map[string]any {
	return map[string]any{
		"entity":            "schema_compatibility_check",
		"id":                item.ID,
		"subject":           item.Subject,
		"candidate_version": item.CandidateVersion,
		"baseline_version":  item.BaselineVersion,
		"check_type":        item.CheckType,
		"compatible":        item.Compatible,
		"summary":           item.Summary,
		"details":           item.Details,
		"created_at":        item.CreatedAt.Unix(),
	}
}

func presentRollout(item Rollout) map[string]any {
	consumers := make([]map[string]any, 0, len(item.Consumers))
	for _, consumer := range item.Consumers {
		consumers = append(consumers, presentRolloutConsumer(consumer))
	}
	resp := map[string]any{
		"entity":       "schema_rollout",
		"id":           item.ID,
		"subject":      item.Subject,
		"from_version": item.FromVersion,
		"to_version":   item.ToVersion,
		"status":       item.Status,
		"notes":        item.Notes,
		"created_at":   item.CreatedAt.Unix(),
		"updated_at":   item.UpdatedAt.Unix(),
		"consumers":    consumers,
	}
	if item.CutoverDeadline != nil {
		resp["cutover_deadline"] = item.CutoverDeadline.Unix()
	}
	return resp
}

func presentRolloutConsumer(item RolloutConsumer) map[string]any {
	return map[string]any{
		"entity":               "schema_rollout_consumer",
		"id":                   item.ID,
		"rollout_id":           item.RolloutID,
		"consumer_name":        item.ConsumerName,
		"acknowledged_version": item.AcknowledgedVersion,
		"acknowledged_at":      item.AcknowledgedAt.Unix(),
		"created_at":           item.CreatedAt.Unix(),
	}
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSchemaNotFound), errors.Is(err, ErrSchemaVersionNotFound), errors.Is(err, ErrSchemaRolloutNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case errors.Is(err, ErrInvalidSchemaDocument), errors.Is(err, ErrInvalidSchemaRollout):
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: err.Error()})
	case errors.Is(err, ErrIncompatibleSchema), errors.Is(err, ErrNoActiveSchemaVersion), errors.Is(err, ErrSchemaAlreadyExists), errors.Is(err, ErrSchemaVersionExists):
		httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "CONFLICT", Description: err.Error()})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error"})
	}
}
