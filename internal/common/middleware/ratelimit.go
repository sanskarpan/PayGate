package middleware

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/time/rate"
)

type RateLimiter struct {
	mu      sync.Mutex
	clients map[string]*rate.Limiter
	r       rate.Limit
	burst   int
}

func NewRateLimiter(rps float64, burst int) *RateLimiter {
	return &RateLimiter{clients: map[string]*rate.Limiter{}, r: rate.Limit(rps), burst: burst}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := rl.rateLimitKey(r)
		limiter := rl.getLimiter(key)
		if !limiter.Allow() {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "RATE_LIMITED", "description": "too many requests"}})
			return
		}
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", float64(rl.r)))
		next.ServeHTTP(w, r)
	})
}

// rateLimitKey returns a key for rate limiting that does NOT include the secret.
// For Basic auth, only the key ID (username portion) is used so the secret is
// never stored in the in-memory map.
func (rl *RateLimiter) rateLimitKey(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "basic ") {
		decoded, err := base64.StdEncoding.DecodeString(auth[6:])
		if err == nil {
			if idx := strings.IndexByte(string(decoded), ':'); idx >= 0 {
				keyID := string(decoded[:idx])
				return keyID + ":" + r.URL.Path
			}
		}
	}
	// Fallback for non-Basic-auth requests (e.g. dashboard session cookie)
	return r.RemoteAddr + ":" + r.URL.Path
}

func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	limiter, ok := rl.clients[key]
	if !ok {
		limiter = rate.NewLimiter(rl.r, rl.burst)
		rl.clients[key] = limiter
	}
	return limiter
}
