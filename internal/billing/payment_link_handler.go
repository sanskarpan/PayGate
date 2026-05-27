package billing

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
)

func (h *Handler) RegisterPublicRoutes(mux *http.ServeMux) {
	mux.Handle("GET /pay/{id}", http.HandlerFunc(h.resolvePaymentLink))
}

func (h *Handler) createPaymentLink(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req CreatePaymentLinkInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	req.MerchantID = p.MerchantID
	out, err := h.svc.CreatePaymentLink(r.Context(), req)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentPaymentLink(out))
}

func (h *Handler) listPaymentLinks(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListPaymentLinks(r.Context(), p.MerchantID, limit)
	if err != nil {
		handleError(w, err)
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, presentPaymentLink(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *Handler) getPaymentLink(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.GetPaymentLink(r.Context(), p.MerchantID, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentPaymentLink(out))
}

func (h *Handler) disablePaymentLink(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.DisablePaymentLink(r.Context(), p.MerchantID, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentPaymentLink(out))
}

func (h *Handler) expirePaymentLink(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.ExpirePaymentLink(r.Context(), p.MerchantID, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentPaymentLink(out))
}

func (h *Handler) resolvePaymentLink(w http.ResponseWriter, r *http.Request) {
	merchantID := r.URL.Query().Get("merchant_id")
	if merchantID == "" {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "merchant_id is required"})
		return
	}
	link, orderResult, err := h.svc.ResolvePaymentLink(r.Context(), merchantID, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	target := "/checkout?merchant_id=" + url.QueryEscape(link.MerchantID) + "&order_id=" + url.QueryEscape(orderResult.ID) + "&callback_url=" + url.QueryEscape(link.CallbackURL)
	http.Redirect(w, r, target, http.StatusFound)
}

func presentPaymentLink(link PaymentLink) map[string]any {
	resp := map[string]any{
		"entity":             "payment_link",
		"id":                 link.ID,
		"merchant_id":        link.MerchantID,
		"customer_id":        link.CustomerID,
		"order_id":           link.OrderID,
		"external_reference": link.ExternalReference,
		"title":              link.Title,
		"description":        link.Description,
		"amount":             link.Amount,
		"currency":           link.Currency,
		"status":             link.Status,
		"callback_url":       link.CallbackURL,
		"notes":              link.Notes,
		"expires_at":         link.ExpiresAt.Unix(),
		"created_at":         link.CreatedAt.Unix(),
		"updated_at":         link.UpdatedAt.Unix(),
		"public_url":         "/pay/" + link.ID + "?merchant_id=" + link.MerchantID,
	}
	if link.LastVisitedAt != nil {
		resp["last_visited_at"] = link.LastVisitedAt.Unix()
	}
	return resp
}
