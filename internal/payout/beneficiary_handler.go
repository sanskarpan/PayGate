package payout

import (
	"encoding/json"
	"net/http"
	"strings"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
)

func (h *Handler) listBeneficiaries(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	items, err := h.svc.ListBeneficiaries(r.Context(), p.MerchantID)
	if err != nil {
		handleError(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, presentBeneficiary(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) createBeneficiary(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		DestinationType   BeneficiaryDestinationType `json:"destination_type"`
		AccountHolderName string                     `json:"account_holder_name"`
		BankAccountNumber string                     `json:"bank_account_number"`
		BankIFSC          string                     `json:"bank_ifsc"`
		VPA               string                     `json:"vpa"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
		return
	}
	last4 := req.BankAccountNumber
	if len(last4) > 4 {
		last4 = last4[len(last4)-4:]
	}
	item, err := h.svc.CreateBeneficiary(r.Context(), Beneficiary{
		MerchantID:        p.MerchantID,
		DestinationType:   req.DestinationType,
		AccountHolderName: strings.TrimSpace(req.AccountHolderName),
		BankAccountLast4:  strings.TrimSpace(last4),
		BankIFSC:          strings.TrimSpace(strings.ToUpper(req.BankIFSC)),
		VPA:               strings.TrimSpace(strings.ToLower(req.VPA)),
	}, p.UserID+p.KeyID, p.Scope)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentBeneficiary(item))
}

func (h *Handler) getBeneficiary(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	item, err := h.svc.GetBeneficiary(r.Context(), p.MerchantID, r.PathValue("beneficiaryID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentBeneficiary(item))
}

func (h *Handler) verifyBeneficiary(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	item, verification, err := h.svc.VerifyBeneficiary(r.Context(), p.MerchantID, r.PathValue("beneficiaryID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity":       "beneficiary_verification_result",
		"beneficiary":  presentBeneficiary(item),
		"verification": presentVerification(verification),
	})
}

func (h *Handler) approveBeneficiary(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Notes string `json:"notes"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
			return
		}
	}
	item, err := h.svc.ApproveBeneficiary(r.Context(), p.MerchantID, r.PathValue("beneficiaryID"), req.Notes, p.UserID+p.KeyID, p.Scope)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentBeneficiary(item))
}

func (h *Handler) approvePayout(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Notes string `json:"notes"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
			return
		}
	}
	out, err := h.svc.ApprovePayout(r.Context(), p.MerchantID, r.PathValue("payoutID"), p.UserID+p.KeyID, p.Scope, req.Notes)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(out))
}

func (h *Handler) rejectPayout(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Notes string `json:"notes"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
			return
		}
	}
	out, err := h.svc.RejectPayout(r.Context(), p.MerchantID, r.PathValue("payoutID"), p.UserID+p.KeyID, p.Scope, req.Notes)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, present(out))
}

func (h *Handler) listApprovals(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	items, err := h.svc.ListApprovals(r.Context(), p.MerchantID, r.PathValue("payoutID"))
	if err != nil {
		handleError(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, map[string]any{
			"entity":      "payout_approval",
			"id":          item.ID,
			"decision":    item.Decision,
			"actor":       item.Actor,
			"actor_scope": item.ActorScope,
			"notes":       item.Notes,
			"created_at":  item.CreatedAt.Unix(),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(payload), "items": payload})
}

func (h *Handler) createBatch(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		DryRun bool `json:"dry_run"`
		Items  []struct {
			SettlementID  string `json:"settlement_id"`
			BeneficiaryID string `json:"beneficiary_id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST", Description: "invalid request body"})
		return
	}
	prefs, err := h.settlement.GetPreferences(r.Context(), p.MerchantID)
	if err != nil {
		handleError(w, err)
		return
	}
	items := make([]BatchItem, 0, len(req.Items))
	for _, item := range req.Items {
		sttl, err := h.settlement.Get(r.Context(), p.MerchantID, strings.TrimSpace(item.SettlementID))
		if err != nil {
			handleError(w, err)
			return
		}
		items = append(items, BatchItem{
			MerchantID:    p.MerchantID,
			SettlementID:  strings.TrimSpace(item.SettlementID),
			BeneficiaryID: strings.TrimSpace(item.BeneficiaryID),
			Amount:        sttl.NetAmount,
			Currency:      sttl.Currency,
			Status:        "preview",
		})
	}
	batch, persistedItems, err := h.svc.CreateBatch(r.Context(), p.MerchantID, r.Header.Get("Idempotency-Key"), req.DryRun, items, prefs.ApprovalThresholdAmount)
	if err != nil {
		handleError(w, err)
		return
	}
	payloadItems := make([]map[string]any, 0, len(persistedItems))
	for _, item := range persistedItems {
		payloadItems = append(payloadItems, map[string]any{
			"id":             item.ID,
			"settlement_id":  item.SettlementID,
			"beneficiary_id": item.BeneficiaryID,
			"payout_id":      item.PayoutID,
			"status":         item.Status,
			"error_text":     item.ErrorText,
		})
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"entity":  "payout_batch",
		"id":      batch.ID,
		"status":  batch.Status,
		"dry_run": batch.DryRun,
		"summary": batch.Summary,
		"items":   payloadItems,
	})
}

func presentBeneficiary(item Beneficiary) map[string]any {
	return map[string]any{
		"entity":                   "beneficiary",
		"id":                       item.ID,
		"destination_type":         item.DestinationType,
		"account_holder_name":      item.AccountHolderName,
		"bank_account_last4":       item.BankAccountLast4,
		"bank_ifsc":                item.BankIFSC,
		"vpa":                      item.VPA,
		"status":                   item.Status,
		"verification_fresh_until": presentTime(item.VerificationFreshUntil),
		"approved_at":              presentTime(item.ApprovedAt),
		"approval_notes":           item.ApprovalNotes,
	}
}

func presentVerification(item BeneficiaryVerification) map[string]any {
	return map[string]any{
		"entity":             "beneficiary_verification",
		"id":                 item.ID,
		"provider":           item.Provider,
		"provider_reference": item.ProviderReference,
		"status":             item.Status,
		"evidence":           item.Evidence,
		"verified_at":        presentTime(item.VerifiedAt),
	}
}
