package payment

import (
	"encoding/json"
	"net/http"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/merchant"
)

func (h *Handler) createNetbankingRedirect(w http.ResponseWriter, r *http.Request) {
	h.createRedirectSession(w, r, "netbanking")
}

func (h *Handler) createWalletRedirect(w http.ResponseWriter, r *http.Request) {
	h.createRedirectSession(w, r, "wallet")
}

func (h *Handler) createRedirectSession(w http.ResponseWriter, r *http.Request, method string) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req CreateRedirectSessionInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	req.MerchantID = p.MerchantID
	req.Method = method
	req.IdempotencyKey = r.Header.Get("Idempotency-Key")
	if req.Currency == "" {
		req.Currency = "INR"
	}
	if h.capabilities != nil {
		if err := h.capabilities.CheckCapability(r.Context(), p.MerchantID, merchant.CapabilityPayments); err != nil {
			handleError(w, err)
			return
		}
	}
	out, err := h.svc.CreateRedirectSession(r.Context(), req)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentRedirectSession(out, redirectCallbackURL(r, out)))
}

func (h *Handler) getRedirectSession(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.GetRedirectSession(r.Context(), p.MerchantID, r.PathValue("paymentID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentRedirectSession(out, redirectCallbackURL(r, out)))
}

func (h *Handler) pollRedirectSession(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.PollRedirectSession(r.Context(), p.MerchantID, r.PathValue("paymentID"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentRedirectSession(out, redirectCallbackURL(r, out)))
}

func (h *Handler) abandonRedirectSession(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
			return
		}
	}
	out, err := h.svc.AbandonRedirectSession(r.Context(), p.MerchantID, r.PathValue("paymentID"), req.Reason)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentRedirectSession(out, redirectCallbackURL(r, out)))
}

func (h *Handler) redirectSessionCallback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MerchantID       string `json:"merchant_id"`
		EventID          string `json:"event_id"`
		Status           string `json:"status"`
		GatewayReference string `json:"gateway_reference"`
		ErrorCode        string `json:"error_code"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	out, processed, err := h.svc.ApplyRedirectCallback(
		r.Context(),
		req.MerchantID,
		r.PathValue("paymentID"),
		r.URL.Query().Get("token"),
		req.EventID,
		req.Status,
		req.GatewayReference,
		req.ErrorCode,
		req.ErrorDescription,
	)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"entity":    "redirect_callback_result",
		"processed": processed,
		"payment":   presentRedirectSession(out, redirectCallbackURL(r, out)),
	})
}

func presentRedirectSession(out RedirectSessionResult, callbackURL string) map[string]any {
	resp := present(out.CaptureResult, nil)
	resp["entity"] = out.Method + "_redirect_payment"
	resp["provider_status"] = out.ProviderStatus
	resp["gateway_reference"] = out.GatewayReference
	resp["bank_code"] = out.BankCode
	resp["bank_name"] = out.BankName
	resp["wallet_code"] = out.WalletCode
	resp["wallet_name"] = out.WalletName
	resp["redirect_url"] = out.RedirectURL
	resp["expires_at"] = out.ExpiresAt.Unix()
	resp["failure_code"] = out.FailureCode
	resp["failure_description"] = out.FailureDescription
	if out.CompletedAt != nil {
		resp["completed_at"] = out.CompletedAt.Unix()
	}
	if out.LastPolledAt != nil {
		resp["last_polled_at"] = out.LastPolledAt.Unix()
	}
	resp["next_action"] = map[string]any{
		"type":         out.Method + "_redirect",
		"redirect_url": out.RedirectURL,
		"expires_at":   out.ExpiresAt.Unix(),
	}
	resp["sandbox"] = map[string]any{
		"callback_url": callbackURL,
	}
	return resp
}

func redirectCallbackURL(r *http.Request, out RedirectSessionResult) string {
	if r == nil || out.PaymentID == "" || out.CallbackToken == "" {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/v1/payments/" + out.PaymentID + "/redirect-session/callbacks?token=" + out.CallbackToken
}
