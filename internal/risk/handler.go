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
	mux.Handle("POST /v1/risk/events/{id}/assign", wrap("admin", http.HandlerFunc(h.assignRiskEvent)))
	mux.Handle("POST /v1/risk/events/{id}/review", wrap("admin", http.HandlerFunc(h.reviewRiskEvent)))
	mux.Handle("GET /v1/risk/config", wrap("read", http.HandlerFunc(h.getFraudConfig)))
	mux.Handle("PUT /v1/risk/config", wrap("admin", http.HandlerFunc(h.upsertFraudConfig)))
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

func (h *Handler) assignRiskEvent(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var body struct {
		AssignedTo string `json:"assigned_to"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.AssignedTo == "" {
		body.AssignedTo = p.UserID
	}
	if err := h.svc.AssignRiskEvent(r.Context(), p.MerchantID, r.PathValue("id"), body.AssignedTo); err != nil {
		if errors.Is(err, ErrRiskEventNotFound) {
			httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: "risk event not found"})
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to assign risk event"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "assigned", "assigned_to": body.AssignedTo})
}

func (h *Handler) reviewRiskEvent(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var body struct {
		Decision string `json:"decision"`
		Notes    string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	if body.Decision != "approve" && body.Decision != "block" {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "decision must be approve or block"})
		return
	}
	ev, err := h.svc.ReviewRiskEvent(r.Context(), p.MerchantID, r.PathValue("id"), body.Decision, body.Notes, p.UserID)
	if err != nil {
		if errors.Is(err, ErrRiskEventNotFound) {
			httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: "risk event not found"})
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to review risk event"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, riskEventToMap(ev))
}

func (h *Handler) getFraudConfig(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	cfg, err := h.svc.GetMerchantFraudConfig(r.Context(), p.MerchantID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to fetch fraud config"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, fraudConfigToMap(cfg))
}

func (h *Handler) upsertFraudConfig(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	// Start from whatever is stored (falling back to defaults) so an omitted
	// field keeps its current value. Decoding into plain ints would zero every
	// key the caller left out, which both destroys the config and violates the
	// positive-value constraints on the table, surfacing as a 500.
	cfg, err := h.svc.GetMerchantFraudConfig(r.Context(), p.MerchantID)
	if err != nil {
		cfg = DefaultMerchantFraudConfig(p.MerchantID)
	}
	var body struct {
		IPVelocityThreshold       *int     `json:"ip_velocity_threshold"`
		DeviceVelocityThreshold   *int     `json:"device_velocity_threshold"`
		MerchantVelocityThreshold *int     `json:"merchant_velocity_threshold"`
		AmountSpikeFactor         *int     `json:"amount_spike_factor"`
		ReviewThreshold           *int     `json:"review_threshold"`
		BlockThreshold            *int     `json:"block_threshold"`
		BlockedCountries          []string `json:"blocked_countries"`
		BlockedBINs               []string `json:"blocked_bins"`
		ReviewOnCountryMismatch   *bool    `json:"review_on_country_mismatch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}

	positive := map[string]*int{
		"ip_velocity_threshold":       body.IPVelocityThreshold,
		"device_velocity_threshold":   body.DeviceVelocityThreshold,
		"merchant_velocity_threshold": body.MerchantVelocityThreshold,
		"amount_spike_factor":         body.AmountSpikeFactor,
	}
	for field, value := range positive {
		if value != nil && *value <= 0 {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: field + " must be greater than zero"})
			return
		}
	}
	nonNegative := map[string]*int{
		"review_threshold": body.ReviewThreshold,
		"block_threshold":  body.BlockThreshold,
	}
	for field, value := range nonNegative {
		if value != nil && *value < 0 {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: field + " must not be negative"})
			return
		}
	}

	if body.IPVelocityThreshold != nil {
		cfg.IPVelocityThreshold = *body.IPVelocityThreshold
	}
	if body.DeviceVelocityThreshold != nil {
		cfg.DeviceVelocityThreshold = *body.DeviceVelocityThreshold
	}
	if body.MerchantVelocityThreshold != nil {
		cfg.MerchantVelocityThreshold = *body.MerchantVelocityThreshold
	}
	if body.AmountSpikeFactor != nil {
		cfg.AmountSpikeFactor = *body.AmountSpikeFactor
	}
	if body.ReviewThreshold != nil {
		cfg.ReviewThreshold = *body.ReviewThreshold
	}
	if body.BlockThreshold != nil {
		cfg.BlockThreshold = *body.BlockThreshold
	}
	if body.BlockedCountries != nil {
		cfg.BlockedCountries = body.BlockedCountries
	}
	if body.BlockedBINs != nil {
		cfg.BlockedBINs = body.BlockedBINs
	}
	if body.ReviewOnCountryMismatch != nil {
		cfg.ReviewOnCountryMismatch = *body.ReviewOnCountryMismatch
	}

	out, err := h.svc.UpsertMerchantFraudConfig(r.Context(), cfg)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to update fraud config"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, fraudConfigToMap(out))
}

func riskEventToMap(ev RiskEvent) map[string]any {
	m := map[string]any{
		"id":                      ev.ID,
		"merchant_id":             ev.MerchantID,
		"payment_id":              ev.PaymentID,
		"score":                   ev.Score,
		"action":                  ev.Action,
		"triggered_rules":         ev.TriggeredRules,
		"device_fingerprint_hash": ev.DeviceFingerprintHash,
		"browser_language":        ev.BrowserLanguage,
		"user_agent":              ev.UserAgent,
		"card_bin":                ev.CardBIN,
		"card_network":            ev.CardNetwork,
		"issuer_country":          ev.IssuerCountry,
		"card_country":            ev.CardCountry,
		"funding_type":            ev.FundingType,
		"review_status":           ev.ReviewStatus,
		"assigned_to":             ev.AssignedTo,
		"review_notes":            ev.ReviewNotes,
		"manual_decision":         ev.ManualDecision,
		"resolved":                ev.Resolved,
		"created_at":              ev.CreatedAt.Unix(),
	}
	if ev.ResolvedBy != "" {
		m["resolved_by"] = ev.ResolvedBy
	}
	if ev.ResolvedAt != nil {
		m["resolved_at"] = ev.ResolvedAt.Unix()
	}
	if ev.AssignedAt != nil {
		m["assigned_at"] = ev.AssignedAt.Unix()
	}
	return m
}

func fraudConfigToMap(cfg MerchantFraudConfig) map[string]any {
	return map[string]any{
		"entity":                      "merchant_fraud_config",
		"merchant_id":                 cfg.MerchantID,
		"ip_velocity_threshold":       cfg.IPVelocityThreshold,
		"device_velocity_threshold":   cfg.DeviceVelocityThreshold,
		"merchant_velocity_threshold": cfg.MerchantVelocityThreshold,
		"amount_spike_factor":         cfg.AmountSpikeFactor,
		"review_threshold":            cfg.ReviewThreshold,
		"block_threshold":             cfg.BlockThreshold,
		"blocked_countries":           cfg.BlockedCountries,
		"blocked_bins":                cfg.BlockedBINs,
		"review_on_country_mismatch":  cfg.ReviewOnCountryMismatch,
	}
}
