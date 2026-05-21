package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"
)

type Config struct {
	Port                   string
	DatabaseURL            string
	RedisAddr              string
	KafkaBrokers           []string
	DashboardSessionSecret string
	DashboardOrigin        string
	TrustedProxyCIDRs      []string
	PayoutRailSecret       string
	SagaWorkerEnabled      bool
}

func FromEnv() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// #nosec G101 -- local development default DSN
		dbURL = "postgres://paygate:paygate@localhost:5435/paygate?sslmode=disable"
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6380"
	}
	kafkaBrokers := strings.TrimSpace(os.Getenv("KAFKA_BROKERS"))
	if kafkaBrokers == "" {
		kafkaBrokers = "localhost:9092"
	}
	sessionSecret := os.Getenv("DASHBOARD_SESSION_SECRET")
	if sessionSecret == "" {
		// #nosec G101 -- development-only fallback secret for local dashboard sessions
		sessionSecret = "paygate-dev-dashboard-session-secret"
	}
	dashboardOrigin := os.Getenv("DASHBOARD_ORIGIN")
	if dashboardOrigin == "" {
		dashboardOrigin = "http://localhost:3001"
	}
	trustedProxyCIDRs := strings.TrimSpace(os.Getenv("TRUSTED_PROXY_CIDRS"))
	if trustedProxyCIDRs == "" {
		trustedProxyCIDRs = "127.0.0.1/32,::1/128"
	}
	payoutRailSecret := os.Getenv("PAYOUT_RAIL_SECRET")
	if payoutRailSecret == "" {
		// #nosec G101 -- development-only fallback secret for local payout rail simulator callbacks
		payoutRailSecret = "paygate-dev-payout-rail-secret"
	}
	sagaWorkerEnabled := os.Getenv("SAGA_WORKER_ENABLED")
	return Config{
		Port:                   port,
		DatabaseURL:            dbURL,
		RedisAddr:              redisAddr,
		KafkaBrokers:           splitCSV(kafkaBrokers),
		DashboardSessionSecret: sessionSecret,
		DashboardOrigin:        dashboardOrigin,
		TrustedProxyCIDRs:      splitCSV(trustedProxyCIDRs),
		PayoutRailSecret:       payoutRailSecret,
		SagaWorkerEnabled:      sagaWorkerEnabled != "false",
	}
}

// Validate checks that all production-critical env vars have been set to
// non-default values. Call this at startup when APP_ENV=production to prevent
// running with insecure development defaults.
func (c Config) Validate() error {
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, fmt.Errorf("DATABASE_URL is required"))
	}
	if c.DashboardSessionSecret == "paygate-dev-dashboard-session-secret" {
		errs = append(errs, fmt.Errorf("DASHBOARD_SESSION_SECRET must be changed from the default in production"))
	}
	if len(c.DashboardSessionSecret) < 32 {
		errs = append(errs, fmt.Errorf("DASHBOARD_SESSION_SECRET must be at least 32 characters"))
	}
	if c.PayoutRailSecret == "paygate-dev-payout-rail-secret" {
		errs = append(errs, fmt.Errorf("PAYOUT_RAIL_SECRET must be changed from the default in production"))
	}
	if len(c.PayoutRailSecret) < 24 {
		errs = append(errs, fmt.Errorf("PAYOUT_RAIL_SECRET must be at least 24 characters"))
	}
	for _, raw := range c.TrustedProxyCIDRs {
		if _, err := netip.ParsePrefix(raw); err != nil {
			errs = append(errs, fmt.Errorf("TRUSTED_PROXY_CIDRS contains invalid CIDR %q", raw))
		}
	}
	return errors.Join(errs...)
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
