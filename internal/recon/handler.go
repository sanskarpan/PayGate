package recon

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
)

// Handler exposes recon query endpoints.
type Handler struct {
	worker *Worker
}

// NewHandler creates a Handler backed by the given Worker.
func NewHandler(w *Worker) *Handler {
	return &Handler{worker: w}
}

// RegisterRoutesWithAuth wires recon read endpoints under authentication.
func (h *Handler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(next http.Handler) http.Handler) {
	mux.Handle("GET /v1/recon/sources", wrap(http.HandlerFunc(h.listSources)))
	mux.Handle("POST /v1/recon/sources/import", wrap(http.HandlerFunc(h.importSource)))
	mux.Handle("GET /v1/recon/mismatches", wrap(http.HandlerFunc(h.listMismatches)))
	mux.Handle("POST /v1/recon/mismatches/{mismatchID}/assign", wrap(http.HandlerFunc(h.assignMismatch)))
	mux.Handle("POST /v1/recon/mismatches/{mismatchID}/resolve", wrap(http.HandlerFunc(h.resolveMismatch)))
	mux.Handle("POST /v1/recon/mismatches/{mismatchID}/reopen", wrap(http.HandlerFunc(h.reopenMismatch)))
	mux.Handle("GET /v1/recon/mismatches/{mismatchID}/notes", wrap(http.HandlerFunc(h.listNotes)))
	mux.Handle("POST /v1/recon/mismatches/{mismatchID}/notes", wrap(http.HandlerFunc(h.addNote)))
}

func (h *Handler) listMismatches(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("count"))
	unresolvedOnly := q.Get("unresolved") == "true"

	mismatches, err := h.worker.ListMismatches(r.Context(), p.MerchantID, limit, unresolvedOnly)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{
			Code:        "SERVER_ERROR",
			Description: "failed to list recon mismatches",
		})
		return
	}

	items := make([]map[string]any, 0, len(mismatches))
	for _, mm := range mismatches {
		items = append(items, map[string]any{
			"id":               mm.ID,
			"batch_id":         mm.BatchID,
			"source_import_id": mm.SourceImportID,
			"mismatch_type":    string(mm.MismatchType),
			"entity_type":      mm.EntityType,
			"entity_id":        mm.EntityID,
			"expected_value":   mm.ExpectedValue,
			"actual_value":     mm.ActualValue,
			"description":      mm.Description,
			"resolved":         mm.Resolved,
			"status":           mm.Status,
			"assigned_to":      mm.AssignedTo,
			"assigned_at":      presentTime(mm.AssignedAt),
			"resolved_at":      presentTime(mm.ResolvedAt),
			"resolved_by":      mm.ResolvedBy,
			"resolution_code":  mm.ResolutionCode,
			"resolution_notes": mm.ResolutionNotes,
			"created_at":       mm.CreatedAt.Unix(),
		})
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity": "collection",
		"count":  len(items),
		"items":  items,
	})
}

func (h *Handler) listSources(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.worker.ListSourceImports(r.Context(), p.MerchantID, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to list recon source imports"})
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, map[string]any{
			"id":             item.ID,
			"batch_id":       item.BatchID,
			"source_type":    item.SourceType,
			"status":         item.Status,
			"period_start":   item.PeriodStart.Unix(),
			"period_end":     item.PeriodEnd.Unix(),
			"entry_count":    item.EntryCount,
			"mismatch_count": item.MismatchCount,
			"created_at":     item.CreatedAt.Unix(),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *Handler) importSource(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		SourceType  string `json:"source_type"`
		PeriodStart int64  `json:"period_start"`
		PeriodEnd   int64  `json:"period_end"`
		Entries     []struct {
			EntityType  string         `json:"entity_type"`
			ExternalID  string         `json:"external_id"`
			ReferenceID string         `json:"reference_id"`
			Amount      int64          `json:"amount"`
			Currency    string         `json:"currency"`
			Status      string         `json:"status"`
			OccurredAt  int64          `json:"occurred_at"`
			Metadata    map[string]any `json:"metadata"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
		return
	}
	entries := make([]SourceEntry, 0, len(req.Entries))
	for _, entry := range req.Entries {
		entries = append(entries, SourceEntry{
			EntityType:  entry.EntityType,
			ExternalID:  entry.ExternalID,
			ReferenceID: entry.ReferenceID,
			Amount:      entry.Amount,
			Currency:    entry.Currency,
			Status:      entry.Status,
			OccurredAt:  time.Unix(entry.OccurredAt, 0).UTC(),
			Metadata:    entry.Metadata,
		})
	}
	item, err := h.worker.ImportSource(r.Context(), ImportSourceInput{
		MerchantID:  p.MerchantID,
		SourceType:  req.SourceType,
		PeriodStart: time.Unix(req.PeriodStart, 0).UTC(),
		PeriodEnd:   time.Unix(req.PeriodEnd, 0).UTC(),
		Entries:     entries,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to import recon source"})
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"entity":         "recon_source_import",
		"id":             item.ID,
		"batch_id":       item.BatchID,
		"source_type":    item.SourceType,
		"status":         item.Status,
		"entry_count":    item.EntryCount,
		"mismatch_count": item.MismatchCount,
		"created_at":     item.CreatedAt.Unix(),
	})
}

func (h *Handler) assignMismatch(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		AssignedTo string `json:"assigned_to"`
		Note       string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
		return
	}
	if err := h.worker.AssignMismatch(r.Context(), p.MerchantID, r.PathValue("mismatchID"), req.AssignedTo, req.Note); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to assign recon mismatch"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "assigned"})
}

func (h *Handler) resolveMismatch(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		ResolutionCode  string `json:"resolution_code"`
		ResolutionNotes string `json:"resolution_notes"`
		Actor           string `json:"actor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
		return
	}
	if err := h.worker.ResolveMismatch(r.Context(), p.MerchantID, r.PathValue("mismatchID"), req.Actor, req.ResolutionCode, req.ResolutionNotes); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to resolve recon mismatch"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "resolved"})
}

func (h *Handler) reopenMismatch(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
		return
	}
	if err := h.worker.ReopenMismatch(r.Context(), p.MerchantID, r.PathValue("mismatchID"), req.Note); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to reopen recon mismatch"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "open"})
}

func (h *Handler) listNotes(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	items, err := h.worker.ListMismatchNotes(r.Context(), p.MerchantID, r.PathValue("mismatchID"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to list recon notes"})
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, map[string]any{
			"id":          item.ID,
			"mismatch_id": item.MismatchID,
			"author":      item.Author,
			"note":        item.Note,
			"created_at":  item.CreatedAt.Unix(),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *Handler) addNote(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Author string `json:"author"`
		Note   string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
		return
	}
	item, err := h.worker.AddMismatchNote(r.Context(), p.MerchantID, r.PathValue("mismatchID"), req.Author, req.Note)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to add recon note"})
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"entity":      "recon_mismatch_note",
		"id":          item.ID,
		"mismatch_id": item.MismatchID,
		"author":      item.Author,
		"note":        item.Note,
		"created_at":  item.CreatedAt.Unix(),
	})
}

func presentTime(ts *time.Time) any {
	if ts == nil {
		return nil
	}
	return ts.Unix()
}
