package order

import (
	"encoding/json"
	"errors"
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

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/orders", h.create)
	mux.HandleFunc("GET /v1/orders/{orderID}", h.get)
	mux.HandleFunc("GET /v1/orders", h.list)
}

func (h *Handler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(scope merchant.APIKeyScope, next http.Handler) http.Handler) {
	mux.Handle("POST /v1/orders", wrap(merchant.APIKeyScopeWrite, http.HandlerFunc(h.create)))
	mux.Handle("GET /v1/orders/{orderID}", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.get)))
	mux.Handle("GET /v1/orders", wrap(merchant.APIKeyScopeRead, http.HandlerFunc(h.list)))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal", Source: "auth", Step: "order_creation", Reason: "missing_principal"})
		return
	}

	var req CreateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body", Source: "business", Step: "order_creation", Reason: "input_validation_failed"})
		return
	}
	if req.Currency == "" {
		req.Currency = "INR"
	}
	req.MerchantID = p.MerchantID
	req.IdempotencyKey = r.Header.Get("Idempotency-Key")

	out, err := h.svc.Create(r.Context(), req)
	if err != nil {
		handleOrderError(w, err, "order_creation")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, presentOrder(out))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal", Source: "auth", Step: "order_fetch", Reason: "missing_principal"})
		return
	}

	orderID := r.PathValue("orderID")
	out, err := h.svc.GetByID(r.Context(), p.MerchantID, orderID)
	if err != nil {
		handleOrderError(w, err, "order_fetch")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, presentOrder(out))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal", Source: "auth", Step: "order_list", Reason: "missing_principal"})
		return
	}

	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, _ := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	cursor := r.URL.Query().Get("cursor")

	out, err := h.svc.List(r.Context(), ListFilter{MerchantID: p.MerchantID, Count: count, From: from, To: to, Cursor: cursor})
	if err != nil {
		handleOrderError(w, err, "order_list")
		return
	}

	items := make([]map[string]any, 0, len(out.Items))
	for _, o := range out.Items {
		items = append(items, presentOrder(o))
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity":      "collection",
		"count":       len(items),
		"items":       items,
		"has_more":    out.HasMore,
		"next_cursor": out.NextCursor,
	})
}

func presentOrder(o Order) map[string]any {
	return map[string]any{
		"id":              o.ID,
		"entity":          "order",
		"amount":          o.Amount,
		"amount_paid":     o.AmountPaid,
		"amount_due":      o.AmountDue,
		"currency":        o.Currency,
		"receipt":         o.Receipt,
		"status":          o.Status,
		"partial_payment": o.PartialPayment,
		"notes":           o.Notes,
		"created_at":      o.CreatedAt.Unix(),
	}
}

func handleOrderError(w http.ResponseWriter, err error, step string) {
	switch {
	case errors.Is(err, ErrInvalidAmount), errors.Is(err, ErrInvalidCurrency):
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: err.Error(), Source: "business", Step: step, Reason: "input_validation_failed"})
	case errors.Is(err, ErrOrderNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{Code: "NOT_FOUND", Description: err.Error(), Source: "business", Step: step, Reason: "resource_missing"})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "internal server error", Source: "internal", Step: step, Reason: "unhandled_exception"})
	}
}
