package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/scrubber"
)

// maxLogBodyBytes is the maximum request body size written to logs.
// Larger bodies are replaced with a placeholder to avoid log bloat and
// potential PII leakage in large JSON payloads.
const maxLogBodyBytes = 4096

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

		var scrubbed string
		if len(body) > maxLogBodyBytes {
			scrubbed = "[BODY_TOO_LARGE]"
		} else {
			scrubbed = scrubber.Scrub(string(body))
		}

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

