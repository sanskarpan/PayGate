package gateway

import (
	"encoding/json"
	"net/http"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
)

// AdminHandler exposes the gateway scenario control panel API.
type AdminHandler struct {
	store *ScenarioStore
}

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(store *ScenarioStore) *AdminHandler {
	return &AdminHandler{store: store}
}

// RegisterRoutes wires gateway admin endpoints into mux.
func (h *AdminHandler) RegisterRoutes(mux *http.ServeMux) {
	h.RegisterRoutesWithAuth(mux, func(next http.Handler) http.Handler { return next })
}

func (h *AdminHandler) RegisterRoutesWithAuth(mux *http.ServeMux, wrap func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/gateway/scenarios", wrap(http.HandlerFunc(h.list)))
	mux.Handle("POST /v1/gateway/scenarios", wrap(http.HandlerFunc(h.upsert)))
	mux.Handle("GET /v1/gateway/scenarios/active", wrap(http.HandlerFunc(h.getActive)))
}

func (h *AdminHandler) list(w http.ResponseWriter, r *http.Request) {
	scenarios, err := h.store.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{
			Code:        "SERVER_ERROR",
			Description: "failed to list scenarios",
		})
		return
	}
	items := make([]map[string]any, 0, len(scenarios))
	for _, sc := range scenarios {
		items = append(items, presentScenario(sc))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity": "collection",
		"count":  len(items),
		"items":  items,
	})
}

func (h *AdminHandler) upsert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MerchantID  string  `json:"merchant_id"`
		Mode        string  `json:"mode"`
		FailureRate float64 `json:"failure_rate"`
		DelayMS     int     `json:"delay_ms"`
		DeclineCode string  `json:"decline_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{
			Code:        "BAD_REQUEST_ERROR",
			Description: "invalid request body",
		})
		return
	}

	mode := ScenarioMode(body.Mode)
	switch mode {
	case ModeSuccess, ModeSlow, ModeFlaky, ModeTimeout, ModeDecline, ModeLateCallback:
		// valid
	default:
		mode = ModeSuccess
	}

	sc := Scenario{
		MerchantID:  body.MerchantID,
		Mode:        mode,
		FailureRate: body.FailureRate,
		DelayMS:     body.DelayMS,
		DeclineCode: body.DeclineCode,
	}
	if sc.DeclineCode == "" {
		sc.DeclineCode = "CARD_DECLINED"
	}

	created, err := h.store.Upsert(r.Context(), sc)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{
			Code:        "SERVER_ERROR",
			Description: "failed to upsert scenario",
		})
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, presentScenario(created))
}

func (h *AdminHandler) getActive(w http.ResponseWriter, r *http.Request) {
	merchantID := r.URL.Query().Get("merchant_id")
	sc, err := h.store.Get(r.Context(), merchantID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{
			Code:        "SERVER_ERROR",
			Description: "failed to get active scenario",
		})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, presentScenario(sc))
}

func presentScenario(sc Scenario) map[string]any {
	return map[string]any{
		"entity":       "gateway_scenario",
		"id":           sc.ID,
		"merchant_id":  sc.MerchantID,
		"mode":         sc.Mode,
		"failure_rate": sc.FailureRate,
		"delay_ms":     sc.DelayMS,
		"decline_code": sc.DeclineCode,
		"active":       sc.Active,
		"created_at":   sc.CreatedAt.Unix(),
	}
}
