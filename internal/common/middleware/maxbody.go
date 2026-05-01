package middleware

import (
	"net/http"
)

// MaxBodyBytes is the default maximum size for request bodies (1 MiB).
const MaxBodyBytes = 1 << 20 // 1 MiB

// MaxBody returns middleware that rejects requests whose body exceeds limit
// bytes with 413 Request Entity Too Large. Passing limit ≤ 0 uses MaxBodyBytes.
func MaxBody(limit int64, next http.Handler) http.Handler {
	if limit <= 0 {
		limit = MaxBodyBytes
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}
