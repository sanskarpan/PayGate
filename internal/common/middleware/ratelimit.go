package middleware

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type RateLimiter struct {
	mu              sync.Mutex
	clients         map[string]*clientLimiter
	r               rate.Limit
	burst           int
	idleTTL         time.Duration
	cleanupInterval time.Duration
	lastCleanup     time.Time
}

func NewRateLimiter(rps float64, burst int) *RateLimiter {
	now := time.Now()
	return &RateLimiter{
		clients:         map[string]*clientLimiter{},
		r:               rate.Limit(rps),
		burst:           burst,
		idleTTL:         15 * time.Minute,
		cleanupInterval: time.Minute,
		lastCleanup:     now,
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := rl.rateLimitKey(r)
		limiter := rl.getLimiter(key)
		if !limiter.Allow() {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "RATE_LIMITED", "description": "too many requests"}})
			return
		}
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", float64(rl.r)))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", maxInt(0, int(math.Floor(limiter.Tokens())))))
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
	// Fallback for non-Basic-auth requests (e.g. dashboard session cookie).
	// Strip the ephemeral TCP port from RemoteAddr so all connections from the
	// same IP share a single token bucket rather than getting one each.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host + ":" + r.URL.Path
}

func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	if now.Sub(rl.lastCleanup) >= rl.cleanupInterval {
		rl.cleanup(now)
	}
	client, ok := rl.clients[key]
	if !ok {
		client = &clientLimiter{limiter: rate.NewLimiter(rl.r, rl.burst)}
		rl.clients[key] = client
	}
	client.lastSeen = now
	return client.limiter
}

func (rl *RateLimiter) cleanup(now time.Time) {
	for key, client := range rl.clients {
		if now.Sub(client.lastSeen) >= rl.idleTTL {
			delete(rl.clients, key)
		}
	}
	rl.lastCleanup = now
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
