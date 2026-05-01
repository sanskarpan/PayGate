package merchant

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/merchants", h.createMerchant)
	mux.HandleFunc("POST /v1/merchants/{merchantID}/keys", h.createAPIKey)
	mux.HandleFunc("DELETE /v1/merchants/{merchantID}/keys/{keyID}", h.revokeAPIKey)
	mux.HandleFunc("POST /v1/merchants/{merchantID}/users/bootstrap", h.bootstrapMerchantUser)
	mux.HandleFunc("POST /v1/dashboard/login", h.dashboardLogin)
	mux.HandleFunc("POST /v1/dashboard/logout", h.dashboardLogout)
}

func (h *Handler) RegisterProtectedRoutes(mux *http.ServeMux, wrap func(scope APIKeyScope, next http.Handler) http.Handler) {
	mux.Handle("GET /v1/dashboard/me", wrap(APIKeyScopeRead, http.HandlerFunc(h.dashboardMe)))
	mux.Handle("GET /v1/merchants/me/api-keys", wrap(APIKeyScopeRead, http.HandlerFunc(h.listOwnAPIKeys)))
	mux.Handle("POST /v1/merchants/me/api-keys", wrap(APIKeyScopeAdmin, http.HandlerFunc(h.createOwnAPIKey)))
	mux.Handle("DELETE /v1/merchants/me/api-keys/{keyID}", wrap(APIKeyScopeAdmin, http.HandlerFunc(h.revokeOwnAPIKey)))
	mux.Handle("POST /v1/merchants/me/api-keys/{keyID}/rotate", wrap(APIKeyScopeAdmin, http.HandlerFunc(h.rotateOwnAPIKey)))
}

func (h *Handler) createMerchant(w http.ResponseWriter, r *http.Request) {
	var req CreateMerchantInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{
			Code:        "BAD_REQUEST_ERROR",
			Description: "invalid request body",
			Source:      "business",
			Step:        "merchant_creation",
			Reason:      "input_validation_failed",
		})
		return
	}

	merchant, err := h.svc.CreateMerchant(r.Context(), req)
	if err != nil {
		handleMerchantError(w, err, "merchant_creation")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":            merchant.ID,
		"entity":        "merchant",
		"name":          merchant.Name,
		"email":         merchant.Email,
		"business_type": merchant.BusinessType,
		"status":        merchant.Status,
		"settings":      merchant.Settings,
		"created_at":    merchant.CreatedAt.Unix(),
	})
}

func (h *Handler) createAPIKey(w http.ResponseWriter, r *http.Request) {
	merchantID := r.PathValue("merchantID")
	if strings.TrimSpace(merchantID) == "" {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{
			Code:        "BAD_REQUEST_ERROR",
			Description: "merchantID path parameter is required",
			Field:       "merchantID",
			Source:      "business",
			Step:        "api_key_creation",
			Reason:      "input_validation_failed",
		})
		return
	}

	var req CreateAPIKeyInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{
			Code:        "BAD_REQUEST_ERROR",
			Description: "invalid request body",
			Source:      "business",
			Step:        "api_key_creation",
			Reason:      "input_validation_failed",
		})
		return
	}

	allowed, err := h.authorizeAPIKeyMutation(r, merchantID)
	if err != nil {
		handleMerchantError(w, err, "api_key_creation")
		return
	}
	if !allowed {
		httpx.WriteError(w, http.StatusForbidden, httpx.APIError{
			Code:        "FORBIDDEN",
			Description: "api key creation requires admin authentication unless bootstrapping the first key",
			Source:      "auth",
			Step:        "api_key_creation",
			Reason:      "insufficient_scope",
		})
		return
	}

	created, err := h.svc.CreateAPIKey(r.Context(), merchantID, req)
	if err != nil {
		handleMerchantError(w, err, "api_key_creation")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"key_id":     created.KeyID,
		"key_secret": created.KeySecret,
		"mode":       created.Mode,
		"scope":      created.Scope,
	})
}

func (h *Handler) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	merchantID := r.PathValue("merchantID")
	keyID := r.PathValue("keyID")

	ok, err := h.requireAdminAPIKey(r, merchantID)
	if err != nil {
		handleMerchantError(w, err, "api_key_revoke")
		return
	}
	if !ok {
		httpx.WriteError(w, http.StatusForbidden, httpx.APIError{
			Code:        "FORBIDDEN",
			Description: "admin api key required to revoke keys",
			Source:      "auth",
			Step:        "api_key_revoke",
			Reason:      "insufficient_scope",
		})
		return
	}

	if err := h.svc.RevokeAPIKey(r.Context(), merchantID, keyID); err != nil {
		handleMerchantError(w, err, "api_key_revoke")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "revoked"})
}

func (h *Handler) bootstrapMerchantUser(w http.ResponseWriter, r *http.Request) {
	merchantID := r.PathValue("merchantID")
	var req BootstrapMerchantUserInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{
			Code:        "BAD_REQUEST_ERROR",
			Description: "invalid request body",
			Source:      "business",
			Step:        "dashboard_user_bootstrap",
			Reason:      "input_validation_failed",
		})
		return
	}
	user, err := h.svc.BootstrapMerchantUser(r.Context(), merchantID, req)
	if err != nil {
		handleMerchantError(w, err, "dashboard_user_bootstrap")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":          user.ID,
		"merchant_id": user.MerchantID,
		"email":       user.Email,
		"role":        user.Role,
		"status":      user.Status,
	})
}

func (h *Handler) dashboardLogin(w http.ResponseWriter, r *http.Request) {
	req, redirectTo, wantsRedirect, err := decodeDashboardLoginRequest(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{
			Code:        "BAD_REQUEST_ERROR",
			Description: "invalid login payload",
			Source:      "business",
			Step:        "dashboard_login",
			Reason:      "input_validation_failed",
		})
		return
	}
	user, err := h.svc.AuthenticateMerchantUser(r.Context(), req.MerchantID, req.Email, req.Password)
	if err != nil {
		handleMerchantError(w, err, "dashboard_login")
		return
	}
	token, err := h.svc.IssueDashboardSession(user)
	if err != nil {
		handleMerchantError(w, err, "dashboard_login")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     DashboardSessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.svc.DashboardSessionTTL().Seconds()),
	})

	if wantsRedirect {
		http.Redirect(w, r, redirectTo, http.StatusSeeOther)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"merchant_id": user.MerchantID,
		"user_id":     user.ID,
		"email":       user.Email,
		"role":        user.Role,
	})
}

func (h *Handler) dashboardLogout(w http.ResponseWriter, r *http.Request) {
	redirectTo := r.URL.Query().Get("redirect_to")
	if redirectTo == "" {
		redirectTo = "/"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     DashboardSessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func (h *Handler) dashboardMe(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"merchant_id": p.MerchantID,
		"user_id":     p.UserID,
		"email":       p.Email,
		"role":        p.Role,
		"scope":       p.Scope,
		"auth_type":   p.AuthType,
	})
}

func (h *Handler) listOwnAPIKeys(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	keys, err := h.svc.ListAPIKeys(r.Context(), p.MerchantID)
	if err != nil {
		handleMerchantError(w, err, "api_key_list")
		return
	}
	items := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		var lastUsedAt int64
		if key.LastUsedAt != nil {
			lastUsedAt = key.LastUsedAt.Unix()
		}
		var revokedAt int64
		if key.RevokedAt != nil {
			revokedAt = key.RevokedAt.Unix()
		}
		items = append(items, map[string]any{
			"id":           key.ID,
			"mode":         key.Mode,
			"scope":        key.Scope,
			"status":       key.Status,
			"last_used_at": lastUsedAt,
			"revoked_at":   revokedAt,
			"created_at":   key.CreatedAt.Unix(),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entity": "collection",
		"count":  len(items),
		"items":  items,
	})
}

func (h *Handler) createOwnAPIKey(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	var req CreateAPIKeyInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "invalid request body"})
		return
	}
	created, err := h.svc.CreateAPIKey(r.Context(), p.MerchantID, req)
	if err != nil {
		handleMerchantError(w, err, "api_key_creation")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"key_id":     created.KeyID,
		"key_secret": created.KeySecret,
		"mode":       created.Mode,
		"scope":      created.Scope,
	})
}

func (h *Handler) revokeOwnAPIKey(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	if err := h.svc.RevokeAPIKey(r.Context(), p.MerchantID, r.PathValue("keyID")); err != nil {
		handleMerchantError(w, err, "api_key_revoke")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *Handler) rotateOwnAPIKey(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{Code: "UNAUTHORIZED", Description: "missing principal"})
		return
	}
	created, err := h.svc.RotateAPIKey(r.Context(), p.MerchantID, r.PathValue("keyID"))
	if err != nil {
		handleMerchantError(w, err, "api_key_rotation")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"key_id":     created.KeyID,
		"key_secret": created.KeySecret,
		"mode":       created.Mode,
		"scope":      created.Scope,
	})
}

func (h *Handler) authorizeAPIKeyMutation(r *http.Request, merchantID string) (bool, error) {
	ok, err := h.requireAdminAPIKey(r, merchantID)
	if err != nil || ok {
		return ok, err
	}
	canBootstrap, err := h.svc.CanBootstrapAPIKey(r.Context(), merchantID)
	if err != nil {
		return false, err
	}
	return canBootstrap, nil
}

func (h *Handler) requireAdminAPIKey(r *http.Request, merchantID string) (bool, error) {
	authHeader := r.Header.Get("Authorization")
	if strings.TrimSpace(authHeader) == "" {
		return false, nil
	}

	keyID, keySecret, ok := parseBasicAuthHeader(authHeader)
	if !ok {
		return false, ErrInvalidCredentials
	}

	key, err := h.svc.AuthenticateAPIKey(r.Context(), keyID, keySecret, APIKeyScopeAdmin)
	if err != nil {
		return false, err
	}
	if key.MerchantID != merchantID {
		return false, ErrScopeNotAllowed
	}
	return true, nil
}

func parseBasicAuthHeader(header string) (string, string, bool) {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "basic") {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", false
	}
	pair := strings.SplitN(string(decoded), ":", 2)
	if len(pair) != 2 || pair[0] == "" || pair[1] == "" {
		return "", "", false
	}
	return pair[0], pair[1], true
}

func handleMerchantError(w http.ResponseWriter, err error, step string) {
	switch {
	case errors.Is(err, ErrInvalidMerchantName),
		errors.Is(err, ErrInvalidMerchantEmail),
		errors.Is(err, ErrInvalidBusinessType),
		errors.Is(err, ErrInvalidAPIKeyMode),
		errors.Is(err, ErrInvalidAPIKeyScope),
		errors.Is(err, ErrInvalidMerchantUser),
		errors.Is(err, ErrInvalidMerchantPass),
		errors.Is(err, ErrInvalidMerchantRole):
		httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{
			Code:        "BAD_REQUEST_ERROR",
			Description: err.Error(),
			Source:      "business",
			Step:        step,
			Reason:      "input_validation_failed",
		})
	case errors.Is(err, ErrMerchantNotFound), errors.Is(err, ErrAPIKeyNotFound), errors.Is(err, ErrMerchantUserNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.APIError{
			Code:        "NOT_FOUND",
			Description: err.Error(),
			Source:      "business",
			Step:        step,
			Reason:      "resource_missing",
		})
	case errors.Is(err, ErrMerchantSuspended),
		errors.Is(err, ErrMerchantDeactivated),
		errors.Is(err, ErrScopeNotAllowed),
		errors.Is(err, ErrMerchantUserNotActive),
		errors.Is(err, ErrBootstrapAlreadyExists):
		httpx.WriteError(w, http.StatusForbidden, httpx.APIError{
			Code:        "FORBIDDEN",
			Description: err.Error(),
			Source:      "business",
			Step:        step,
			Reason:      "merchant_state_restricted",
		})
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrDashboardSession):
		httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{
			Code:        "UNAUTHORIZED",
			Description: err.Error(),
			Source:      "auth",
			Step:        step,
			Reason:      "invalid_credentials",
		})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{
			Code:        "SERVER_ERROR",
			Description: "internal server error",
			Source:      "internal",
			Step:        step,
			Reason:      "unhandled_exception",
		})
	}
}

type dashboardLoginRequest struct {
	MerchantID string `json:"merchant_id"`
	Email      string `json:"email"`
	Password   string `json:"password"`
}

func decodeDashboardLoginRequest(r *http.Request) (dashboardLoginRequest, string, bool, error) {
	redirectTo := r.URL.Query().Get("redirect_to")
	switch {
	case strings.Contains(r.Header.Get("Content-Type"), "application/json"):
		var req dashboardLoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return dashboardLoginRequest{}, "", false, err
		}
		return req, redirectTo, false, nil
	default:
		if err := r.ParseForm(); err != nil {
			return dashboardLoginRequest{}, "", false, err
		}
		if redirectTo == "" {
			redirectTo = r.FormValue("redirect_to")
		}
		if redirectTo == "" {
			redirectTo = "/orders"
		}
		if _, err := url.ParseRequestURI(redirectTo); err != nil {
			redirectTo = "/orders"
		}
		return dashboardLoginRequest{
			MerchantID: r.FormValue("merchant_id"),
			Email:      r.FormValue("email"),
			Password:   r.FormValue("password"),
		}, redirectTo, true, nil
	}
}
