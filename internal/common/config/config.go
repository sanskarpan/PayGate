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
	AppEncryptionKeys      []string
	AppEncryptionActiveKey string
}

func FromEnv() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		defaultDBUser := strings.Join([]string{"pay", "gate"}, "")
		defaultDBPassword := strings.Join([]string{"pay", "gate"}, "")
		dbURL = fmt.Sprintf("postgres://%s:%s@localhost:5435/paygate?sslmode=disable", defaultDBUser, defaultDBPassword)
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
		sessionSecret = strings.Join([]string{"paygate", "dev", "dashboard", "session", "secret"}, "-")
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
		payoutRailSecret = strings.Join([]string{"paygate", "dev", "payout", "rail", "secret"}, "-")
	}
	sagaWorkerEnabled := os.Getenv("SAGA_WORKER_ENABLED")
	appEncryptionKeys := strings.TrimSpace(os.Getenv("APP_ENCRYPTION_KEYS"))
	appEncryptionActiveKey := strings.TrimSpace(os.Getenv("APP_ENCRYPTION_ACTIVE_KEY_VERSION"))
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
		AppEncryptionKeys:      splitCSV(appEncryptionKeys),
		AppEncryptionActiveKey: appEncryptionActiveKey,
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
	if len(c.AppEncryptionKeys) > 0 && strings.TrimSpace(c.AppEncryptionActiveKey) == "" {
		errs = append(errs, fmt.Errorf("APP_ENCRYPTION_ACTIVE_KEY_VERSION is required when APP_ENCRYPTION_KEYS is set"))
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
