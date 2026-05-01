// Package ratelimit provides tier-based per-merchant rate limiting.
// Three tiers are defined: free, standard, and enterprise, with
// increasing RPS and burst allowances.
package ratelimit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/time/rate"
)

// Tier identifies a merchant rate-limit tier.
type Tier string

const (
	TierFree       Tier = "free"
	TierStandard   Tier = "standard"
	TierEnterprise Tier = "enterprise"
)

// TierConfig holds the token-bucket parameters for one tier.
type TierConfig struct {
	// RPS is the sustained requests-per-second limit.
	RPS float64
	// Burst is the maximum burst above the sustained rate.
	Burst int
}

// DefaultTierConfigs returns the default rate-limit configuration for
// each tier. Callers may override individual tiers before passing to
// NewTieredLimiter.
func DefaultTierConfigs() map[Tier]TierConfig {
	return map[Tier]TierConfig{
		TierFree:       {RPS: 10, Burst: 20},
		TierStandard:   {RPS: 100, Burst: 200},
		TierEnterprise: {RPS: 1000, Burst: 2000},
	}
}

// TierFunc returns the Tier for a given API key ID. Implementations
// may query a cache or database. If the key is unknown, TierFree
// should be returned so the caller is never unprotected.
type TierFunc func(keyID string) Tier

// TieredLimiter is a per-key rate limiter whose bucket parameters are
// determined by the key's tier. It is safe for concurrent use.
type TieredLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	configs  map[Tier]TierConfig
	tierFn   TierFunc
}

// NewTieredLimiter creates a TieredLimiter with the given tier configs
// and tier-lookup function. If configs is nil, DefaultTierConfigs() is
// used. If tierFn is nil, all keys are treated as TierFree.
func NewTieredLimiter(configs map[Tier]TierConfig, tierFn TierFunc) *TieredLimiter {
	if configs == nil {
		configs = DefaultTierConfigs()
	}
	if tierFn == nil {
		tierFn = func(string) Tier { return TierFree }
	}
	return &TieredLimiter{
		limiters: make(map[string]*rate.Limiter),
		configs:  configs,
		tierFn:   tierFn,
	}
}

// Middleware returns an http.Handler that enforces per-key tier-based
// rate limits. On rejection it writes 429 JSON with a Retry-After: 1 header.
func (tl *TieredLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keyID := extractKeyID(r)
		limiter := tl.getLimiter(keyID)
		if !limiter.Allow() {
			tier := tl.tierFn(keyID)
			cfg := tl.configForTier(tier)
			w.Header().Set("Retry-After", "1")
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", cfg.RPS))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":        "RATE_LIMITED",
					"description": "too many requests",
				},
			})
			return
		}
		cfg := tl.configForTier(tl.tierFn(keyID))
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", cfg.RPS))
		next.ServeHTTP(w, r)
	})
}

func (tl *TieredLimiter) getLimiter(keyID string) *rate.Limiter {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	if l, ok := tl.limiters[keyID]; ok {
		return l
	}
	tier := tl.tierFn(keyID)
	cfg := tl.configForTier(tier)
	l := rate.NewLimiter(rate.Limit(cfg.RPS), cfg.Burst)
	tl.limiters[keyID] = l
	return l
}

func (tl *TieredLimiter) configForTier(t Tier) TierConfig {
	if cfg, ok := tl.configs[t]; ok {
		return cfg
	}
	return tl.configs[TierFree]
}

// extractKeyID returns the API key ID from an HTTP Basic Auth header.
// Only the username (key ID) is extracted; the secret is never stored.
// Falls back to RemoteAddr for unauthenticated requests.
func extractKeyID(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "basic ") {
		decoded, err := base64.StdEncoding.DecodeString(auth[6:])
		if err == nil {
			if idx := strings.IndexByte(string(decoded), ':'); idx >= 0 {
				return string(decoded[:idx])
			}
		}
	}
	return r.RemoteAddr
}
