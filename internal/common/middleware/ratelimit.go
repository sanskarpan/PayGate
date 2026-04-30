package middleware

import (
	"encoding/json"
	"net/http"
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
		key := r.Header.Get("Authorization") + ":" + r.URL.Path
		limiter := rl.getLimiter(key)
		if !limiter.Allow() {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "RATE_LIMITED", "description": "too many requests"}})
			return
		}
		w.Header().Set("X-RateLimit-Limit", "25")
		w.Header().Set("X-RateLimit-Remaining", "unknown")
		next.ServeHTTP(w, r)
	})
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
