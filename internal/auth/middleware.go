package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
	"github.com/sanskarpan/PayGate/internal/merchant"
)

type Verifier interface {
	AuthenticateAPIKey(ctx context.Context, keyID, keySecret string, requiredScope merchant.APIKeyScope) (merchant.APIKey, error)
	AuthenticateDashboardSession(ctx context.Context, token string, requiredScope merchant.APIKeyScope) (merchant.MerchantUser, error)
}

type Middleware struct {
	verifier Verifier
}

func NewMiddleware(verifier Verifier) *Middleware {
	return &Middleware{verifier: verifier}
}

func (m *Middleware) RequireScope(required merchant.APIKeyScope, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keyID, keySecret, ok := parseBasicAuth(r.Header.Get("Authorization"))
		if ok {
			key, err := m.verifier.AuthenticateAPIKey(r.Context(), keyID, keySecret, required)
			if err != nil {
				writeAuthError(w, err)
				return
			}

			ctx := httpx.WithPrincipal(r.Context(), httpx.Principal{
				MerchantID: key.MerchantID,
				KeyID:      key.ID,
				Scope:      string(key.Scope),
				AuthType:   "api_key",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		sessionCookie, err := r.Cookie(merchant.DashboardSessionCookieName)
		if err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, httpx.APIError{
				Code:        "UNAUTHORIZED",
				Description: "missing or invalid authorization header",
				Source:      "auth",
				Step:        "authentication",
				Reason:      "invalid_credentials",
			})
			return
		}

		user, err := m.verifier.AuthenticateDashboardSession(r.Context(), sessionCookie.Value, required)
		if err != nil {
			writeAuthError(w, err)
			return
		}

		ctx := httpx.WithPrincipal(r.Context(), httpx.Principal{
			MerchantID: user.MerchantID,
			UserID:     user.ID,
			Email:      user.Email,
			Role:       string(user.Role),
			Scope:      string(merchant.ScopeForMerchantUserRole(user.Role)),
			AuthType:   "dashboard_session",
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeAuthError(w http.ResponseWriter, err error) {
	status := http.StatusUnauthorized
	code := "UNAUTHORIZED"
	reason := "invalid_credentials"
	if errors.Is(err, merchant.ErrScopeNotAllowed) {
		status = http.StatusForbidden
		code = "FORBIDDEN"
		reason = "insufficient_scope"
	}
	httpx.WriteError(w, status, httpx.APIError{
		Code:        code,
		Description: err.Error(),
		Source:      "auth",
		Step:        "authentication",
		Reason:      reason,
	})
}

func parseBasicAuth(header string) (string, string, bool) {
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
