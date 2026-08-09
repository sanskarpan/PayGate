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
// The body is already size-limited by MaxBody, and this runs inside the rate
// limiter, so the buffering here is bounded per client. The error is written
// directly rather than through the shared httpx helper, which would introduce
// an import cycle.
func RejectNullBytes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Path arrives decoded; RawQuery does not, so %00 has to be caught
		// before net/url decodes it into a NUL.
		if strings.ContainsRune(r.URL.Path, 0) ||
			strings.ContainsRune(r.URL.RawQuery, 0) ||
			strings.Contains(strings.ToLower(r.URL.RawQuery), "%00") {
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
		if bytes.IndexByte(body, 0) >= 0 || containsNullEscape(body) {
			writeNullByteError(w)
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

// containsNullEscape reports whether the body contains a JSON unicode escape
// for U+0000 that would actually decode to a NUL byte.
//
// The length of the backslash run decides this. One backslash before "u0000"
// is a live escape and decodes to NUL. Two means the first backslash escapes
// the second, so what remains is the literal text "u0000" — valid input that
// must not be rejected. Only an odd-length run is a real escape.
func containsNullEscape(body []byte) bool {
	lowered := bytes.ToLower(body)
	for i := 0; ; {
		idx := bytes.Index(lowered[i:], []byte("u0000"))
		if idx < 0 {
			return false
		}
		pos := i + idx
		backslashes := 0
		for j := pos - 1; j >= 0 && lowered[j] == '\\'; j-- {
			backslashes++
		}
		if backslashes%2 == 1 {
			return true
		}
		i = pos + len("u0000")
	}
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
