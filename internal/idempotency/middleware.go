package idempotency

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	httpx "github.com/sanskarpan/PayGate/internal/common/http"
)

type Middleware struct {
	store *Store
}

func NewMiddleware(store *Store) *Middleware {
	return &Middleware{store: store}
}

func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m == nil || m.store == nil || r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}

		clientKey := r.Header.Get("Idempotency-Key")
		if clientKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		principal, ok := httpx.PrincipalFromContext(r.Context())
		if !ok || principal.MerchantID == "" {
			next.ServeHTTP(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, httpx.APIError{Code: "BAD_REQUEST_ERROR", Description: "unable to read request body"})
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		endpointHash, requestHash := HashRequest(r.Method, r.URL.Path, body)
		decision, err := m.store.Start(r.Context(), principal.MerchantID, endpointHash, clientKey, requestHash)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, httpx.APIError{Code: "SERVER_ERROR", Description: "idempotency check failed"})
			return
		}
		if decision.Conflict {
			w.Header().Set("Retry-After", "30")
			httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "IDEMPOTENCY_CONFLICT", Description: "idempotency key was reused with a different request"})
			return
		}
		if decision.InProgress {
			w.Header().Set("Retry-After", strconv.Itoa(decision.RetryAfter))
			httpx.WriteError(w, http.StatusConflict, httpx.APIError{Code: "REQUEST_IN_PROGRESS", Description: "request with this idempotency key is already in progress"})
			return
		}
		if decision.Replay {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(decision.ResponseCode)
			_, _ = w.Write(decision.ResponseBody)
			return
		}

		recorder := newResponseRecorder(w)
		next.ServeHTTP(recorder, r)
		resourceType, resourceID := resourceFromBody(recorder.body.Bytes())
		if recorder.statusCode >= http.StatusInternalServerError {
			_ = m.store.Fail(r.Context(), principal.MerchantID, endpointHash, clientKey, requestHash, recorder.statusCode, recorder.body.Bytes())
			return
		}
		_ = m.store.Complete(r.Context(), principal.MerchantID, endpointHash, clientKey, requestHash, resourceType, resourceID, recorder.statusCode, recorder.body.Bytes())
	})
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseRecorder) Write(body []byte) (int, error) {
	r.body.Write(body)
	return r.ResponseWriter.Write(body)
}

func resourceFromBody(body []byte) (string, string) {
	if len(body) == 0 {
		return "", ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", ""
	}
	entity, _ := payload["entity"].(string)
	resourceID, _ := payload["id"].(string)
	if resourceID == "" {
		resourceID, _ = payload["key_id"].(string)
	}
	return entity, resourceID
}
