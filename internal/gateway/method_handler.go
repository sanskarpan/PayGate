package gateway

import (
	"encoding/json"
	"net/http"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
)

// MethodHandler exposes the gateway method-config API.
type MethodHandler struct {
	store *MethodConfigStore
}

// NewMethodHandler creates a new MethodHandler.
func NewMethodHandler(store *MethodConfigStore) *MethodHandler {
	return &MethodHandler{store: store}
}

// RegisterRoutes wires method-config endpoints into mux.
func (h *MethodHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/gateway/method-configs", h.list)
	mux.HandleFunc("POST /v1/gateway/method-configs", h.upsert)
}

func (h *MethodHandler) list(w http.ResponseWriter, r *http.Request) {
	configs, err := h.store.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{
			Code:        "SERVER_ERROR",
			Description: "failed to list method configs",
		})
		return
	}
	items := make([]map[string]any, 0, len(configs))
	for _, cfg := range configs {
		items = append(items, presentMethodConfig(cfg))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity": "collection",
		"count":  len(items),
		"items":  items,
	})
}

func (h *MethodHandler) upsert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MerchantID  string  `json:"merchant_id"`
		Method      string  `json:"method"`
		Enabled     *bool   `json:"enabled"`
		SuccessRate float64 `json:"success_rate"`
		DeclineCode string  `json:"decline_code"`
		DelayMS     int     `json:"delay_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{
			Code:        "BAD_REQUEST_ERROR",
			Description: "invalid request body",
		})
		return
	}

	// Validate method.
	method := PaymentMethod(body.Method)
	switch method {
	case MethodCard, MethodUPI, MethodNetbanking, MethodWallet:
		// valid
	default:
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{
			Code:        "BAD_REQUEST_ERROR",
			Description: "method must be one of: card, upi, netbanking, wallet",
		})
		return
	}

	// Default enabled to true if not provided.
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	// Default success_rate to 1.0 if not provided.
	successRate := body.SuccessRate
	if successRate == 0 {
		successRate = 1.0
	}

	cfg := MethodConfig{
		MerchantID:  body.MerchantID,
		Method:      method,
		Enabled:     enabled,
		SuccessRate: successRate,
		DeclineCode: body.DeclineCode,
		DelayMS:     body.DelayMS,
	}

	created, err := h.store.Upsert(r.Context(), cfg)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{
			Code:        "SERVER_ERROR",
			Description: "failed to upsert method config",
		})
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentMethodConfig(created))
}

func presentMethodConfig(cfg MethodConfig) map[string]any {
	return map[string]any{
		"entity":       "gateway_method_config",
		"id":           cfg.ID,
		"merchant_id":  cfg.MerchantID,
		"method":       cfg.Method,
		"enabled":      cfg.Enabled,
		"success_rate": cfg.SuccessRate,
		"decline_code": cfg.DeclineCode,
		"delay_ms":     cfg.DelayMS,
		"created_at":   cfg.CreatedAt.Unix(),
	}
}
