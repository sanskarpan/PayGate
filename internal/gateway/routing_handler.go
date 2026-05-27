package gateway

import (
	"encoding/json"
	"net/http"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
)

type RoutingHandler struct {
	store *RoutingPolicyStore
}

func NewRoutingHandler(store *RoutingPolicyStore) *RoutingHandler {
	return &RoutingHandler{store: store}
}

func (h *RoutingHandler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/gateway/routing-policies", wrap(http.HandlerFunc(h.list)))
	mux.Handle("POST /v1/gateway/routing-policies", wrap(http.HandlerFunc(h.upsert)))
}

func (h *RoutingHandler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to list routing policies"})
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		resp = append(resp, presentRoutingPolicy(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entity": "collection", "count": len(resp), "items": resp})
}

func (h *RoutingHandler) upsert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MerchantID        string `json:"merchant_id"`
		Method            string `json:"method"`
		PrimaryProvider   string `json:"primary_provider"`
		FallbackProvider  string `json:"fallback_provider"`
		ForceProvider     string `json:"force_provider"`
		FailoverOnDecline bool   `json:"failover_on_decline"`
		FailoverOnError   bool   `json:"failover_on_error"`
		CostWeight        int    `json:"cost_weight"`
		SuccessWeight     int    `json:"success_weight"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	method := PaymentMethod(body.Method)
	switch method {
	case MethodCard, MethodUPI, MethodNetbanking, MethodWallet:
	default:
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "unsupported method"})
		return
	}
	out, err := h.store.Upsert(r.Context(), RoutingPolicy{
		MerchantID:        body.MerchantID,
		Method:            method,
		PrimaryProvider:   body.PrimaryProvider,
		FallbackProvider:  body.FallbackProvider,
		ForceProvider:     body.ForceProvider,
		FailoverOnDecline: body.FailoverOnDecline,
		FailoverOnError:   body.FailoverOnError,
		CostWeight:        body.CostWeight,
		SuccessWeight:     body.SuccessWeight,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "failed to upsert routing policy"})
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentRoutingPolicy(out))
}

func presentRoutingPolicy(policy RoutingPolicy) map[string]any {
	return map[string]any{
		"entity":              "gateway_routing_policy",
		"id":                  policy.ID,
		"merchant_id":         policy.MerchantID,
		"method":              policy.Method,
		"primary_provider":    policy.PrimaryProvider,
		"fallback_provider":   policy.FallbackProvider,
		"force_provider":      policy.ForceProvider,
		"failover_on_decline": policy.FailoverOnDecline,
		"failover_on_error":   policy.FailoverOnError,
		"cost_weight":         policy.CostWeight,
		"success_weight":      policy.SuccessWeight,
		"created_at":          policy.CreatedAt.Unix(),
	}
}
