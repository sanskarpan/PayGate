package config

import "testing"

func TestFromEnvUsesDockerBackedLocalDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("KAFKA_BROKERS", "")
	t.Setenv("DASHBOARD_SESSION_SECRET", "")
	t.Setenv("DASHBOARD_ORIGIN", "")
	t.Setenv("TRUSTED_PROXY_CIDRS", "")

	cfg := FromEnv()

	if cfg.Port != "8090" {
		t.Fatalf("expected default port 8090, got %q", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://paygate:paygate@localhost:5435/paygate?sslmode=disable" {
		t.Fatalf("unexpected default database url %q", cfg.DatabaseURL)
	}
	if cfg.RedisAddr != "localhost:6380" {
		t.Fatalf("unexpected default redis addr %q", cfg.RedisAddr)
	}
	if len(cfg.KafkaBrokers) != 1 || cfg.KafkaBrokers[0] != "localhost:9092" {
		t.Fatalf("unexpected default kafka brokers %#v", cfg.KafkaBrokers)
	}
	if cfg.DashboardOrigin != "http://localhost:3001" {
		t.Fatalf("unexpected default dashboard origin %q", cfg.DashboardOrigin)
	}
}
