package middleware

import (
	"net/http"

	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

const requestIDHeader = "X-Request-Id"

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(requestIDHeader)
		if requestID == "" {
			requestID = idgen.New("req")
		}
		w.Header().Set(requestIDHeader, requestID)
		next.ServeHTTP(w, r)
	})
}
