package billing

import (
	"encoding/json"
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
	mux.Handle("POST /v1/customers", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.createCustomer)))
	mux.Handle("GET /v1/customers", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listCustomers)))
	mux.Handle("GET /v1/customers/{id}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getCustomer)))
	mux.Handle("PATCH /v1/customers/{id}", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.updateCustomer)))

	mux.Handle("POST /v1/upi-mandates", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.createUPIMandate)))
	mux.Handle("GET /v1/upi-mandates", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listUPIMandates)))
	mux.Handle("GET /v1/upi-mandates/{id}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getUPIMandate)))
	mux.Handle("GET /v1/upi-mandates/{id}/events", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listUPIMandateEvents)))
	mux.Handle("POST /v1/upi-mandates/{id}/activate", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.activateUPIMandate)))
	mux.Handle("POST /v1/upi-mandates/{id}/pause", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.pauseUPIMandate)))
	mux.Handle("POST /v1/upi-mandates/{id}/resume", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.resumeUPIMandate)))
	mux.Handle("POST /v1/upi-mandates/{id}/revoke", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.revokeUPIMandate)))
	mux.Handle("POST /v1/upi-mandates/{id}/expire", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.expireUPIMandate)))

	mux.Handle("POST /v1/virtual-accounts", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.createVirtualAccount)))
	mux.Handle("GET /v1/virtual-accounts", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listVirtualAccounts)))
	mux.Handle("GET /v1/virtual-accounts/{id}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getVirtualAccount)))
	mux.Handle("POST /v1/virtual-accounts/inbound", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.recordInboundCollection)))
	mux.Handle("GET /v1/collections", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listInboundCollections)))
	mux.Handle("POST /v1/collections/{id}/review", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.reviewInboundCollection)))

	mux.Handle("POST /v1/connected-accounts", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.createConnectedAccount)))
	mux.Handle("GET /v1/connected-accounts", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listConnectedAccounts)))
	mux.Handle("GET /v1/connected-accounts/{id}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getConnectedAccount)))

	mux.Handle("POST /v1/subscriptions", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.createSubscription)))
	mux.Handle("GET /v1/subscriptions", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listSubscriptions)))
	mux.Handle("GET /v1/subscriptions/{id}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getSubscription)))
	mux.Handle("POST /v1/subscriptions/{id}/pause", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.pauseSubscription)))
	mux.Handle("POST /v1/subscriptions/{id}/resume", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.resumeSubscription)))
	mux.Handle("POST /v1/subscriptions/{id}/cancel", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.cancelSubscription)))
	mux.Handle("POST /v1/subscriptions/{id}/run", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.runSubscription)))
	mux.Handle("POST /v1/subscriptions/run-due", wrap(merchant.APIKeyScopeAdmin, http.HandlerFunc(h.runDueSubscriptions)))

	mux.Handle("GET /v1/invoices", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.listInvoices)))
	mux.Handle("GET /v1/invoices/{id}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.getInvoice)))
}

func (h *Handler) createVirtualAccount(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req CreateVirtualAccountInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	req.MerchantID = p.MerchantID
	out, err := h.svc.CreateVirtualAccount(r.Context(), req)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentVirtualAccount(out))
}

func (h *Handler) listVirtualAccounts(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListVirtualAccounts(r.Context(), p.MerchantID, limit)
	if err != nil {
		handleError(w, err)
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, presentVirtualAccount(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *Handler) getVirtualAccount(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.GetVirtualAccount(r.Context(), p.MerchantID, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentVirtualAccount(out))
}

func (h *Handler) recordInboundCollection(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req RecordInboundCollectionInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	req.MerchantID = p.MerchantID
	out, err := h.svc.RecordInboundCollection(r.Context(), req)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentCollection(out))
}

func (h *Handler) listInboundCollections(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	reviewOnly := r.URL.Query().Get("review_only") == "true"
	items, err := h.svc.ListInboundCollections(r.Context(), p.MerchantID, limit, reviewOnly)
	if err != nil {
		handleError(w, err)
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, presentCollection(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *Handler) reviewInboundCollection(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		OrderID    string `json:"order_id"`
		CustomerID string `json:"customer_id"`
		Notes      string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	out, err := h.svc.ReviewInboundCollection(r.Context(), p.MerchantID, r.PathValue("id"), req.OrderID, req.CustomerID, req.Notes)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentCollection(out))
}

func (h *Handler) createConnectedAccount(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req CreateConnectedAccountInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	req.MerchantID = p.MerchantID
	out, err := h.svc.CreateConnectedAccount(r.Context(), req)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentConnectedAccount(out))
}

func (h *Handler) listConnectedAccounts(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListConnectedAccounts(r.Context(), p.MerchantID, limit)
	if err != nil {
		handleError(w, err)
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, presentConnectedAccount(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *Handler) getConnectedAccount(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.GetConnectedAccount(r.Context(), p.MerchantID, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentConnectedAccount(out))
}

func (h *Handler) createCustomer(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req CreateCustomerInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	req.MerchantID = p.MerchantID
	out, err := h.svc.CreateCustomer(r.Context(), req)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentCustomer(out))
}

func (h *Handler) createUPIMandate(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req CreateUPIMandateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	req.MerchantID = p.MerchantID
	out, err := h.svc.CreateUPIMandate(r.Context(), req, p.AuthType, firstNonEmpty(p.UserID, p.KeyID))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentUPIMandate(out))
}

func (h *Handler) listUPIMandates(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListUPIMandates(r.Context(), p.MerchantID, limit)
	if err != nil {
		handleError(w, err)
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, presentUPIMandate(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *Handler) getUPIMandate(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.GetUPIMandate(r.Context(), p.MerchantID, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentUPIMandate(out))
}

func (h *Handler) listUPIMandateEvents(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListUPIMandateEvents(r.Context(), p.MerchantID, r.PathValue("id"), limit)
	if err != nil {
		handleError(w, err)
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, presentUPIMandateEvent(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *Handler) activateUPIMandate(w http.ResponseWriter, r *http.Request) {
	h.transitionUPIMandate(w, r, "activate")
}

func (h *Handler) pauseUPIMandate(w http.ResponseWriter, r *http.Request) {
	h.transitionUPIMandate(w, r, "pause")
}

func (h *Handler) resumeUPIMandate(w http.ResponseWriter, r *http.Request) {
	h.transitionUPIMandate(w, r, "resume")
}

func (h *Handler) revokeUPIMandate(w http.ResponseWriter, r *http.Request) {
	h.transitionUPIMandate(w, r, "revoke")
}

func (h *Handler) expireUPIMandate(w http.ResponseWriter, r *http.Request) {
	h.transitionUPIMandate(w, r, "expire")
}

func (h *Handler) transitionUPIMandate(w http.ResponseWriter, r *http.Request, action string) {
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
	actorType := p.AuthType
	actorID := firstNonEmpty(p.UserID, p.KeyID)
	var (
		out UPIMandate
		err error
	)
	switch action {
	case "activate", "resume":
		out, err = h.svc.ActivateUPIMandate(r.Context(), p.MerchantID, r.PathValue("id"), actorType, actorID, req.Reason)
	case "pause":
		out, err = h.svc.PauseUPIMandate(r.Context(), p.MerchantID, r.PathValue("id"), actorType, actorID, req.Reason)
	case "revoke":
		out, err = h.svc.RevokeUPIMandate(r.Context(), p.MerchantID, r.PathValue("id"), actorType, actorID, req.Reason)
	case "expire":
		out, err = h.svc.ExpireUPIMandate(r.Context(), p.MerchantID, r.PathValue("id"), actorType, actorID, req.Reason)
	}
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentUPIMandate(out))
}

func (h *Handler) listCustomers(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListCustomers(r.Context(), p.MerchantID, limit)
	if err != nil {
		handleError(w, err)
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, presentCustomer(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *Handler) getCustomer(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.GetCustomer(r.Context(), p.MerchantID, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentCustomer(out))
}

func (h *Handler) updateCustomer(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	current, err := h.svc.GetCustomer(r.Context(), p.MerchantID, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	var req struct {
		Name                  string         `json:"name"`
		Email                 string         `json:"email"`
		Phone                 string         `json:"phone"`
		ExternalReference     string         `json:"external_reference"`
		DefaultPaymentTokenID string         `json:"default_payment_token_id"`
		Metadata              map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	current.Name = req.Name
	current.Email = req.Email
	current.Phone = req.Phone
	current.ExternalReference = req.ExternalReference
	current.DefaultPaymentTokenID = req.DefaultPaymentTokenID
	current.Metadata = req.Metadata
	out, err := h.svc.UpdateCustomer(r.Context(), current)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentCustomer(out))
}

func (h *Handler) createSubscription(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req CreateSubscriptionInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	req.MerchantID = p.MerchantID
	out, err := h.svc.CreateSubscription(r.Context(), req)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentSubscription(out))
}

func (h *Handler) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListSubscriptions(r.Context(), p.MerchantID, limit)
	if err != nil {
		handleError(w, err)
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, presentSubscription(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *Handler) getSubscription(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.GetSubscription(r.Context(), p.MerchantID, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentSubscription(out))
}

func (h *Handler) pauseSubscription(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	out, err := h.svc.PauseSubscription(r.Context(), p.MerchantID, r.PathValue("id"), req.Reason)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentSubscription(out))
}

func (h *Handler) resumeSubscription(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.ResumeSubscription(r.Context(), p.MerchantID, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentSubscription(out))
}

func (h *Handler) cancelSubscription(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req struct {
		AtPeriodEnd bool `json:"at_period_end"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	out, err := h.svc.CancelSubscription(r.Context(), p.MerchantID, r.PathValue("id"), req.AtPeriodEnd)
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentSubscription(out))
}

func (h *Handler) runSubscription(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	invoice, capture, err := h.svc.RunSubscription(r.Context(), p.MerchantID, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity":  "subscription_run_result",
		"invoice": presentInvoice(invoice),
		"payment": map[string]any{"id": capture.PaymentID, "status": capture.Status, "captured": capture.Captured, "order_id": capture.OrderID},
	})
}

func (h *Handler) runDueSubscriptions(w http.ResponseWriter, r *http.Request) {
	_, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	invoices, err := h.svc.RunDueSubscriptions(r.Context(), 25)
	if err != nil {
		handleError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(invoices))
	for _, invoice := range invoices {
		items = append(items, presentInvoice(invoice))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(items), "items": items})
}

func (h *Handler) listInvoices(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("count"))
	items, err := h.svc.ListInvoices(r.Context(), p.MerchantID, r.URL.Query().Get("subscription_id"), limit)
	if err != nil {
		handleError(w, err)
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, presentInvoice(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *Handler) getInvoice(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	out, err := h.svc.GetInvoice(r.Context(), p.MerchantID, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentInvoice(out))
}

func presentCustomer(customer Customer) map[string]any {
	return map[string]any{
		"entity":                   "customer",
		"id":                       customer.ID,
		"merchant_id":              customer.MerchantID,
		"name":                     customer.Name,
		"email":                    customer.Email,
		"phone":                    customer.Phone,
		"external_reference":       customer.ExternalReference,
		"default_payment_token_id": customer.DefaultPaymentTokenID,
		"metadata":                 customer.Metadata,
		"created_at":               customer.CreatedAt.Unix(),
		"updated_at":               customer.UpdatedAt.Unix(),
	}
}

func presentUPIMandate(mandate UPIMandate) map[string]any {
	resp := map[string]any{
		"entity":             "upi_mandate",
		"id":                 mandate.ID,
		"merchant_id":        mandate.MerchantID,
		"customer_id":        mandate.CustomerID,
		"reference":          mandate.Reference,
		"display_name":       mandate.DisplayName,
		"vpa":                mandate.VPA,
		"amount_limit":       mandate.AmountLimit,
		"currency":           mandate.Currency,
		"interval_unit":      mandate.IntervalUnit,
		"interval_count":     mandate.IntervalCount,
		"retry_window_hours": mandate.RetryWindowHours,
		"status":             mandate.Status,
		"metadata":           mandate.Metadata,
		"created_at":         mandate.CreatedAt.Unix(),
		"updated_at":         mandate.UpdatedAt.Unix(),
		"sandbox": map[string]any{
			"approval_token": mandate.ApprovalToken,
		},
	}
	if mandate.ApprovedAt != nil {
		resp["approved_at"] = mandate.ApprovedAt.Unix()
	}
	if mandate.PausedAt != nil {
		resp["paused_at"] = mandate.PausedAt.Unix()
	}
	if mandate.RevokedAt != nil {
		resp["revoked_at"] = mandate.RevokedAt.Unix()
	}
	if mandate.ExpiresAt != nil {
		resp["expires_at"] = mandate.ExpiresAt.Unix()
	}
	if mandate.LatestVerificationID != "" {
		verification := map[string]any{
			"id":       mandate.LatestVerificationID,
			"version":  mandate.LatestVerificationVersion,
			"status":   mandate.LatestVerificationStatus,
			"provider": mandate.LatestVerificationProvider,
		}
		if mandate.LatestVerificationVerifiedAt != nil {
			verification["verified_at"] = mandate.LatestVerificationVerifiedAt.Unix()
		}
		if mandate.LatestVerificationExpiresAt != nil {
			verification["expires_at"] = mandate.LatestVerificationExpiresAt.Unix()
		}
		resp["latest_verification"] = verification
	}
	return resp
}

func presentUPIMandateEvent(event UPIMandateEvent) map[string]any {
	return map[string]any{
		"entity":      "upi_mandate_event",
		"id":          event.ID,
		"merchant_id": event.MerchantID,
		"mandate_id":  event.MandateID,
		"event_type":  event.EventType,
		"actor_type":  event.ActorType,
		"actor_id":    event.ActorID,
		"reason":      event.Reason,
		"payment_id":  event.PaymentID,
		"metadata":    event.Metadata,
		"created_at":  event.CreatedAt.Unix(),
	}
}

func presentVirtualAccount(account VirtualAccount) map[string]any {
	return map[string]any{
		"entity":         "virtual_account",
		"id":             account.ID,
		"merchant_id":    account.MerchantID,
		"customer_id":    account.CustomerID,
		"order_id":       account.OrderID,
		"reference":      account.Reference,
		"provider":       account.Provider,
		"bank_name":      account.BankName,
		"account_number": account.AccountNumber,
		"ifsc":           account.IFSC,
		"upi_vpa":        account.UPIVPA,
		"status":         account.Status,
		"metadata":       account.Metadata,
		"created_at":     account.CreatedAt.Unix(),
		"updated_at":     account.UpdatedAt.Unix(),
	}
}

func presentCollection(collection InboundCollection) map[string]any {
	resp := map[string]any{
		"entity":             "inbound_collection",
		"id":                 collection.ID,
		"merchant_id":        collection.MerchantID,
		"virtual_account_id": collection.VirtualAccountID,
		"customer_id":        collection.CustomerID,
		"order_id":           collection.OrderID,
		"amount":             collection.Amount,
		"currency":           collection.Currency,
		"remitter_name":      collection.RemitterName,
		"remitter_account":   collection.RemitterAccount,
		"remitter_ifsc":      collection.RemitterIFSC,
		"remitter_vpa":       collection.RemitterVPA,
		"utr":                collection.UTR,
		"status":             collection.Status,
		"review_notes":       collection.ReviewNotes,
		"created_at":         collection.CreatedAt.Unix(),
		"updated_at":         collection.UpdatedAt.Unix(),
	}
	if collection.MatchedAt != nil {
		resp["matched_at"] = collection.MatchedAt.Unix()
	}
	return resp
}

func presentConnectedAccount(account ConnectedAccount) map[string]any {
	return map[string]any{
		"entity":             "connected_account",
		"id":                 account.ID,
		"merchant_id":        account.MerchantID,
		"linked_merchant_id": account.LinkedMerchantID,
		"beneficiary_id":     account.BeneficiaryID,
		"display_name":       account.DisplayName,
		"external_reference": account.ExternalReference,
		"status":             account.Status,
		"metadata":           account.Metadata,
		"created_at":         account.CreatedAt.Unix(),
		"updated_at":         account.UpdatedAt.Unix(),
	}
}

func presentSubscription(subscription Subscription) map[string]any {
	resp := map[string]any{
		"entity":                  "subscription",
		"id":                      subscription.ID,
		"merchant_id":             subscription.MerchantID,
		"customer_id":             subscription.CustomerID,
		"plan_name":               subscription.PlanName,
		"collection_method":       subscription.CollectionMethod,
		"payment_method_token_id": subscription.PaymentMethodTokenID,
		"upi_mandate_id":          subscription.UPIMandateID,
		"amount":                  subscription.Amount,
		"currency":                subscription.Currency,
		"interval_unit":           subscription.IntervalUnit,
		"interval_count":          subscription.IntervalCount,
		"status":                  subscription.Status,
		"next_billing_at":         subscription.NextBillingAt.Unix(),
		"retry_count":             subscription.RetryCount,
		"max_retry_count":         subscription.MaxRetryCount,
		"retry_interval_hours":    subscription.RetryIntervalHours,
		"cancel_at_period_end":    subscription.CancelAtPeriodEnd,
		"pause_reason":            subscription.PauseReason,
		"metadata":                subscription.Metadata,
		"created_at":              subscription.CreatedAt.Unix(),
		"updated_at":              subscription.UpdatedAt.Unix(),
	}
	if subscription.CanceledAt != nil {
		resp["canceled_at"] = subscription.CanceledAt.Unix()
	}
	return resp
}

func presentInvoice(invoice Invoice) map[string]any {
	return map[string]any{
		"entity":          "invoice",
		"id":              invoice.ID,
		"merchant_id":     invoice.MerchantID,
		"customer_id":     invoice.CustomerID,
		"subscription_id": invoice.SubscriptionID,
		"amount":          invoice.Amount,
		"currency":        invoice.Currency,
		"status":          invoice.Status,
		"billing_reason":  invoice.BillingReason,
		"period_start":    invoice.PeriodStart.Unix(),
		"period_end":      invoice.PeriodEnd.Unix(),
		"due_at":          invoice.DueAt.Unix(),
		"order_id":        invoice.OrderID,
		"payment_id":      invoice.PaymentID,
		"failure_code":    invoice.FailureCode,
		"failure_message": invoice.FailureMessage,
		"created_at":      invoice.CreatedAt.Unix(),
		"updated_at":      invoice.UpdatedAt.Unix(),
	}
}

func handleError(w http.ResponseWriter, err error) {
	switch err {
	case ErrCustomerNotFound, ErrVirtualAccountNotFound, ErrCollectionNotFound, ErrConnectedAccountNotFound, ErrSubscriptionNotFound, ErrInvoiceNotFound, ErrUPIMandateNotFound:
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error()})
	case ErrInvalidSubscription, ErrInvalidVirtualAccount, ErrInvalidCollection, ErrInvalidConnectedAccount, ErrInvalidSplitInstruction, ErrCardTokenNotReusable, ErrCustomerTokenMismatch, ErrSubscriptionNotActive, ErrInvalidUPIMandate, ErrUPIMandateNotActive, ErrMandateCustomerMismatch, ErrVPAVerificationRequired:
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: err.Error()})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: err.Error()})
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
