package recon

import (
	"net/http"
	"strconv"

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
	mux.Handle("GET /v1/recon/mismatches", wrap(http.HandlerFunc(h.listMismatches)))
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
			"id":             mm.ID,
			"batch_id":       mm.BatchID,
			"mismatch_type":  string(mm.MismatchType),
			"entity_type":    mm.EntityType,
			"entity_id":      mm.EntityID,
			"expected_value": mm.ExpectedValue,
			"actual_value":   mm.ActualValue,
			"description":    mm.Description,
			"resolved":       mm.Resolved,
			"created_at":     mm.CreatedAt.Unix(),
		})
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity": "collection",
		"count":  len(items),
		"items":  items,
	})
}
