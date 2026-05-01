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

// scrubRe matches JSON keys whose values should be redacted from logs.
var scrubRe = regexp.MustCompile(`(?i)(password|secret|token|cvv|card_number|email|phone|pan|account_number|card_no)`)

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
			scrubbed = scrub(string(body))
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

// scrub replaces values of sensitive JSON keys with [SCRUBBED].
// It operates on each line of the body so that compact single-line JSON and
// pretty-printed multi-line JSON are both handled.  When a line contains a
// sensitive key name anywhere, the whole line is replaced rather than
// attempting to parse the JSON (which avoids regex-based false negatives).
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
