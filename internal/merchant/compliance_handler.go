package merchant

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
)

func (h *Handler) listOnboardingParties(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	items, err := h.svc.ListOnboardingParties(r.Context(), p.MerchantID)
	if err != nil {
		handleMerchantError(w, err, "merchant_onboarding_parties_list")
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentOnboardingParty(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) replaceOnboardingParties(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Items []UpsertOnboardingPartyInput `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	items, err := h.svc.ReplaceOnboardingParties(r.Context(), p.MerchantID, req.Items, actorIdentity(p), stringScope(p))
	if err != nil {
		handleMerchantError(w, err, "merchant_onboarding_parties_replace")
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentOnboardingParty(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) listOnboardingDocuments(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	items, err := h.svc.ListOnboardingDocuments(r.Context(), p.MerchantID)
	if err != nil {
		handleMerchantError(w, err, "merchant_onboarding_documents_list")
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentOnboardingDocument(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) requestOnboardingDocument(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req RequestDocumentInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	item, err := h.svc.RequestOnboardingDocument(r.Context(), p.MerchantID, req, actorIdentity(p), stringScope(p))
	if err != nil {
		handleMerchantError(w, err, "merchant_onboarding_document_request")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentOnboardingDocument(item))
}

func (h *Handler) uploadOnboardingDocument(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req UploadDocumentInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	item, err := h.svc.UploadOnboardingDocument(r.Context(), p.MerchantID, req, actorIdentity(p), stringScope(p))
	if err != nil {
		handleMerchantError(w, err, "merchant_onboarding_document_upload")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentOnboardingDocument(item))
}

func (h *Handler) reviewOnboardingDocument(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Status      DocumentStatus `json:"status"`
		ReviewNotes string         `json:"review_notes"`
		ExpiresAt   string         `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	var expiresAt *time.Time
	if strings.TrimSpace(req.ExpiresAt) != "" {
		ts, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "expires_at must be RFC3339"})
			return
		}
		expiresAt = &ts
	}
	item, err := h.svc.ReviewOnboardingDocument(r.Context(), p.MerchantID, r.PathValue("documentID"), req.Status, req.ReviewNotes, expiresAt, actorIdentity(p), stringScope(p))
	if err != nil {
		handleMerchantError(w, err, "merchant_onboarding_document_review")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentOnboardingDocument(item))
}

func (h *Handler) listScreeningCases(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	items, err := h.svc.ListScreeningCases(r.Context(), p.MerchantID)
	if err != nil {
		handleMerchantError(w, err, "merchant_screening_list")
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentScreeningCase(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) runScreening(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req RunScreeningInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && r.ContentLength > 0 {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	item, err := h.svc.RunScreening(r.Context(), p.MerchantID, req, actorIdentity(p), stringScope(p))
	if err != nil {
		handleMerchantError(w, err, "merchant_screening_run")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentScreeningCase(item))
}

func (h *Handler) listCapabilities(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	items, err := h.svc.ListCapabilities(r.Context(), p.MerchantID)
	if err != nil {
		handleMerchantError(w, err, "merchant_capabilities_list")
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentCapability(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) updateCapabilities(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Items []CapabilityUpdateInput `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	items, err := h.svc.UpdateCapabilities(r.Context(), p.MerchantID, req.Items, actorIdentity(p))
	if err != nil {
		handleMerchantError(w, err, "merchant_capabilities_update")
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentCapability(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) getReservePolicy(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	policy, err := h.svc.GetReservePolicy(r.Context(), p.MerchantID)
	if err != nil {
		handleMerchantError(w, err, "merchant_reserve_policy_get")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentReservePolicy(policy))
}

func (h *Handler) updateReservePolicy(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req UpsertReservePolicyInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	policy, err := h.svc.UpsertReservePolicy(r.Context(), p.MerchantID, req, actorIdentity(p))
	if err != nil {
		handleMerchantError(w, err, "merchant_reserve_policy_update")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentReservePolicy(policy))
}

func (h *Handler) listReserveEscalations(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	status := ReserveEscalationStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	items, err := h.svc.ListReserveEscalations(r.Context(), p.MerchantID, status)
	if err != nil {
		handleMerchantError(w, err, "merchant_reserve_escalation_list")
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentReserveEscalation(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) reviewReserveEscalation(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Decision       ReserveEscalationStatus   `json:"decision"`
		Notes          string                    `json:"notes"`
		PolicyOverride *UpsertReservePolicyInput `json:"policy_override"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	if req.Decision != ReserveEscalationApproved && req.Decision != ReserveEscalationRejected {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "decision must be approved or rejected"})
		return
	}
	item, err := h.svc.ReviewReserveEscalation(r.Context(), p.MerchantID, r.PathValue("id"), req.Decision, req.Notes, actorIdentity(p), req.PolicyOverride)
	if err != nil {
		handleMerchantError(w, err, "merchant_reserve_escalation_review")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentReserveEscalation(item))
}

func presentOnboardingParty(item OnboardingParty) map[string]any {
	return map[string]any{
		"entity":              "merchant_onboarding_party",
		"id":                  item.ID,
		"party_type":          item.PartyType,
		"full_name":           item.FullName,
		"title":               item.Title,
		"email":               item.Email,
		"phone":               item.Phone,
		"ownership_bps":       item.OwnershipBPS,
		"verification_status": item.VerificationStatus,
		"evidence_notes":      item.EvidenceNotes,
		"revision":            item.Revision,
	}
}

func presentOnboardingDocument(item OnboardingDocument) map[string]any {
	return map[string]any{
		"entity":         "merchant_onboarding_document",
		"id":             item.ID,
		"document_type":  item.DocumentType,
		"file_name":      item.FileName,
		"content_type":   item.ContentType,
		"storage_key":    item.StorageKey,
		"request_reason": item.RequestReason,
		"review_notes":   item.ReviewNotes,
		"status":         item.Status,
		"expires_at":     presentRFC3339(item.ExpiresAt),
	}
}

func presentScreeningCase(item ScreeningCase) map[string]any {
	return map[string]any{
		"entity":             "merchant_screening_case",
		"id":                 item.ID,
		"screening_type":     item.ScreeningType,
		"provider":           item.Provider,
		"provider_reference": item.ProviderReference,
		"subject_name":       item.SubjectName,
		"status":             item.Status,
		"result_payload":     item.ResultPayload,
		"screened_at":        item.ScreenedAt.Unix(),
	}
}

func presentCapability(item MerchantCapability) map[string]any {
	return map[string]any{
		"entity":          "merchant_capability",
		"id":              item.ID,
		"capability_code": item.CapabilityCode,
		"status":          item.Status,
		"reason":          item.Reason,
	}
}

func presentReservePolicy(item ReservePolicy) map[string]any {
	return map[string]any{
		"entity":           "merchant_reserve_policy",
		"id":               item.ID,
		"policy_type":      item.PolicyType,
		"percentage_bps":   item.PercentageBPS,
		"hold_days":        item.HoldDays,
		"threshold_amount": item.ThresholdAmount,
		"notes":            item.Notes,
	}
}

func presentReserveEscalation(item ReserveEscalation) map[string]any {
	resp := map[string]any{
		"entity":                     "merchant_reserve_escalation",
		"id":                         item.ID,
		"merchant_id":                item.MerchantID,
		"risk_event_id":              item.RiskEventID,
		"trigger_score":              item.TriggerScore,
		"triggered_rules":            item.TriggeredRules,
		"status":                     item.Status,
		"suggested_policy_type":      item.SuggestedPolicyType,
		"suggested_percentage_bps":   item.SuggestedPercentageBPS,
		"suggested_hold_days":        item.SuggestedHoldDays,
		"suggested_threshold_amount": item.SuggestedThresholdAmount,
		"rationale":                  item.Rationale,
		"review_notes":               item.ReviewNotes,
		"reviewed_by":                item.ReviewedBy,
		"created_at":                 item.CreatedAt.Unix(),
		"updated_at":                 item.UpdatedAt.Unix(),
	}
	if item.ReviewedAt != nil {
		resp["reviewed_at"] = item.ReviewedAt.Unix()
	}
	return resp
}

func presentRFC3339(ts *time.Time) any {
	if ts == nil {
		return nil
	}
	return ts.UTC().Format(time.RFC3339)
}

func stringScope(p httpx.Principal) string {
	if strings.TrimSpace(p.Scope) == "" {
		return "unknown"
	}
	return p.Scope
}
