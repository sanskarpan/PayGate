package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// RejectNullBytes refuses requests carrying a NUL byte in the URL or body.
//
// PostgreSQL cannot store U+0000 in a text column, so such a value reaches the
// driver and comes back as `invalid byte sequence for encoding "UTF8": 0x00`.
// That surfaced as an unhandled 500 on any endpoint persisting caller-supplied
// text — including unauthenticated merchant creation — which is a caller input
// error, not a server fault.
//
// The body is already size-limited by MaxBody, so buffering it here is bounded.
// The error is written directly rather than through the shared httpx helper,
// which would introduce an import cycle.
func RejectNullBytes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.ContainsRune(r.URL.Path, 0) || strings.ContainsRune(r.URL.RawQuery, 0) {
			writeNullByteError(w)
			return
		}

		if r.Body == nil || r.ContentLength == 0 {
			next.ServeHTTP(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err != nil {
			// Let the handler's own decoder report a malformed or oversized body.
			r.Body = io.NopCloser(bytes.NewReader(body))
			next.ServeHTTP(w, r)
			return
		}
		// A raw 0x00 byte, or the JSON escape that decodes into one. The escape
		// is the common case: encoders emit \u0000 rather than a literal NUL.
		if bytes.IndexByte(body, 0) >= 0 || containsNullEscape(body) {
			writeNullByteError(w)
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

// containsNullEscape reports whether the JSON body carries a \u0000 escape in
// any case form (\u0000, \U0000).
func containsNullEscape(body []byte) bool {
	return bytes.Contains(bytes.ToLower(body), []byte(`\u0000`))
}

func writeNullByteError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":        "BAD_REQUEST_ERROR",
			"description": "request must not contain null bytes",
			"source":      "business",
			"step":        "input_validation",
			"reason":      "input_validation_failed",
		},
	})
}
