package reporting

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
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
	mux.Handle("GET /v1/reports/catalog", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.catalog)))
	mux.Handle("POST /v1/reports/statements/{entityType}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.statement)))
	mux.Handle("POST /v1/reports/exports", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.requestExport)))
	mux.Handle("GET /v1/reports/exports", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listExports)))
	mux.Handle("GET /v1/reports/exports/{exportID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getExport)))
	mux.Handle("GET /v1/reports/exports/{exportID}/download", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.downloadExport)))
	mux.Handle("GET /v1/reports/tax-profile", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getTaxProfile)))
	mux.Handle("PUT /v1/reports/tax-profile", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.putTaxProfile)))
}

func (h *Handler) catalog(w http.ResponseWriter, _ *http.Request) {
	items := h.svc.Catalog()
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, map[string]any{
			"report_type":  item.ReportType,
			"label":        item.Label,
			"description":  item.Description,
			"supports_api": item.SupportsAPIs,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *Handler) statement(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	start, end, ok := parsePeriodBody(w, r)
	if !ok {
		return
	}
	stmt, err := h.svc.BuildStatement(r.Context(), p.MerchantID, ReportType(r.PathValue("entityType")), start, end)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentStatement(stmt))
}

func (h *Handler) requestExport(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		ReportType  ReportType `json:"report_type"`
		Format      string     `json:"format"`
		PeriodStart int64      `json:"period_start"`
		PeriodEnd   int64      `json:"period_end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
		return
	}
	job, err := h.svc.RequestExport(r.Context(), ExportRequest{
		MerchantID:  p.MerchantID,
		ReportType:  req.ReportType,
		Format:      req.Format,
		PeriodStart: time.Unix(req.PeriodStart, 0).UTC(),
		PeriodEnd:   time.Unix(req.PeriodEnd, 0).UTC(),
	})
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentExport(job, r))
}

func (h *Handler) listExports(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListExports(r.Context(), p.MerchantID, limit)
	if err != nil {
		handleError(w, err)
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, presentExport(item, r))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *Handler) getExport(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	item, err := h.svc.GetExport(r.Context(), p.MerchantID, r.PathValue("exportID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentExport(item, r))
}

func (h *Handler) downloadExport(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	item, err := h.svc.GetExport(r.Context(), p.MerchantID, r.PathValue("exportID"))
	if err != nil {
		handleError(w, err)
		return
	}
	if r.URL.Query().Get("token") != item.DownloadToken || time.Now().UTC().After(item.DownloadExpiresAt) {
		httpx.WriteError(w, http.StatusForbidden, httpx.APIError{Code: "INVALID_DOWNLOAD_TOKEN", Description: "download token is invalid or expired"})
		return
	}
	w.Header().Set("Content-Type", item.ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+item.FileName+`"`)
	_, _ = w.Write([]byte(item.ContentText))
}

func (h *Handler) getTaxProfile(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	item, err := h.svc.GetTaxProfile(r.Context(), p.MerchantID)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentTaxProfile(item))
}

func (h *Handler) putTaxProfile(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req TaxProfile
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
		return
	}
	req.MerchantID = p.MerchantID
	item, err := h.svc.UpsertTaxProfile(r.Context(), req)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentTaxProfile(item))
}

func parsePeriodBody(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, bool) {
	var req struct {
		PeriodStart int64 `json:"period_start"`
		PeriodEnd   int64 `json:"period_end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
		return time.Time{}, time.Time{}, false
	}
	start := time.Unix(req.PeriodStart, 0).UTC()
	end := time.Unix(req.PeriodEnd, 0).UTC()
	if !end.After(start) {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "period_end must be after period_start"})
		return time.Time{}, time.Time{}, false
	}
	return start, end, true
}

func presentStatement(item Statement) map[string]any {
	rows := make([]map[string]any, 0, len(item.Rows))
	for _, row := range item.Rows {
		out := make(map[string]any, len(row))
		for k, v := range row {
			if ts, ok := v.(time.Time); ok {
				out[k] = ts.Unix()
				out[k+"_rfc3339"] = ts.Format(time.RFC3339)
				continue
			}
			out[k] = v
		}
		rows = append(rows, out)
	}
	return map[string]any{
		"entity":       "statement",
		"entity_type":  item.EntityType,
		"period_start": item.PeriodStart.Unix(),
		"period_end":   item.PeriodEnd.Unix(),
		"tax_rate_bps": item.TaxRateBPS,
		"totals":       item.Totals,
		"rows":         rows,
	}
}

func presentExport(item ExportJob, r *http.Request) map[string]any {
	downloadURL := ""
	if item.DownloadToken != "" {
		downloadURL = "http://" + r.Host + "/v1/reports/exports/" + item.ID + "/download?token=" + item.DownloadToken
		if r.TLS != nil {
			downloadURL = "https://" + r.Host + "/v1/reports/exports/" + item.ID + "/download?token=" + item.DownloadToken
		}
	}
	var completedAt any
	if item.CompletedAt != nil {
		completedAt = item.CompletedAt.Unix()
	}
	return map[string]any{
		"entity":              "export_job",
		"id":                  item.ID,
		"report_type":         item.ReportType,
		"format":              item.Format,
		"status":              item.Status,
		"file_name":           item.FileName,
		"content_type":        item.ContentType,
		"file_size_bytes":     item.FileSizeBytes,
		"filters":             item.Filters,
		"download_url":        downloadURL,
		"download_expires_at": item.DownloadExpiresAt.Unix(),
		"error_message":       item.ErrorMessage,
		"created_at":          item.CreatedAt.Unix(),
		"completed_at":        completedAt,
	}
}

func presentTaxProfile(item TaxProfile) map[string]any {
	return map[string]any{
		"entity":               "tax_profile",
		"merchant_id":          item.MerchantID,
		"legal_name":           item.LegalName,
		"gstin":                item.GSTIN,
		"business_state_code":  item.BusinessStateCode,
		"place_of_supply":      item.PlaceOfSupply,
		"default_tax_rate_bps": item.DefaultTaxRateBPS,
		"created_at":           item.CreatedAt.Unix(),
		"updated_at":           item.UpdatedAt.Unix(),
	}
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrExportNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error"})
	}
}
