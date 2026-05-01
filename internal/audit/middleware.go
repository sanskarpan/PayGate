package audit

import (
	"net/http"
	"strings"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
)

// Middleware returns an http.Handler wrapper that records an audit log entry
// for every mutating request (POST, PUT, PATCH, DELETE) that completes with a
// 2xx status code.  Read-only requests (GET, HEAD, OPTIONS) are skipped.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip non-mutating methods.
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			next.ServeHTTP(w, r)
			return
		}

		rw := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rw, r)

		// Only audit successful mutations.
		if rw.status < 200 || rw.status >= 300 {
			return
		}

		p, ok := httpx.PrincipalFromContext(r.Context())
		if !ok {
			return
		}

		actorType := ActorTypeAPIKey
		if p.AuthType == "dashboard_user" {
			actorType = ActorTypeDashboardUser
		}

		resourceType, resourceID := extractResource(r.URL.Path)
		action := r.Method + " " + resourceType

		// Collect remote IP; prefer X-Forwarded-For when behind a proxy.
		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = strings.TrimSpace(strings.SplitN(fwd, ",", 2)[0])
		}

		corrID := r.Header.Get("X-Request-Id")

		s.Record(r.Context(), RecordInput{
			MerchantID:    p.MerchantID,
			ActorID:       firstNonEmpty(p.UserID, p.KeyID),
			ActorEmail:    p.Email,
			ActorType:     actorType,
			Action:        action,
			ResourceType:  resourceType,
			ResourceID:    resourceID,
			IPAddress:     ip,
			CorrelationID: corrID,
		})
	})
}

// extractResource heuristically extracts resource type and ID from a URL path.
// "/v1/payments/pay_abc123/refunds" → ("payments.refunds", "pay_abc123")
func extractResource(path string) (resourceType, resourceID string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// Strip /v1 prefix
	if len(parts) > 0 && parts[0] == "v1" {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return "unknown", ""
	}
	switch len(parts) {
	case 1:
		return parts[0], ""
	case 2:
		return parts[0], parts[1]
	default:
		// e.g. payments/{id}/refunds → resource = payments.refunds, id = parts[1]
		return parts[0] + "." + parts[len(parts)-1], parts[1]
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// statusRecorder captures the HTTP status code written by the next handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}
