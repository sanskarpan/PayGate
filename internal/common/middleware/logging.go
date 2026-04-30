package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var scrubRe = regexp.MustCompile(`(?i)(password|secret|token|cvv|card_number)`)

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func Logging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if logger == nil {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}

		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewBuffer(body))
		scrubbed := scrub(string(body))

		next.ServeHTTP(rec, r)
		logger.Info("http_request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_body", scrubbed,
		)
	})
}

func scrub(v string) string {
	if v == "" {
		return v
	}
	parts := strings.Split(v, "\n")
	for i := range parts {
		if scrubRe.MatchString(parts[i]) {
			parts[i] = "[SCRUBBED]"
		}
	}
	return strings.Join(parts, "\n")
}
