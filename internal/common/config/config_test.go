package config

import "testing"

func TestFromEnvUsesDockerBackedLocalDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "")
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
	expectedDatabaseURL := "postgres://" + "paygate" + ":" + "paygate" + "@localhost:5435/paygate?sslmode=disable"
	if cfg.DatabaseURL != expectedDatabaseURL {
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
	if cfg.AppEnv != "development" {
		t.Fatalf("unexpected default app env %q", cfg.AppEnv)
	}
	if cfg.AppEncryptionProvider != "disabled" {
		t.Fatalf("unexpected default encryption provider %q", cfg.AppEncryptionProvider)
	}
}

func TestValidateAllowsEnvEncryptionProviderInProduction(t *testing.T) {
	cfg := Config{
		AppEnv:                 "production",
		DatabaseURL:            "postgres://localhost:5435/paygate?sslmode=disable",
		DashboardSessionSecret: "abcdefghijklmnopqrstuvwxyz012345",
		PayoutRailSecret:       "abcdefghijklmnopqrstuvwxyz",
		TrustedProxyCIDRs:      []string{"127.0.0.1/32"},
		AppEncryptionProvider:  "env",
		AppEncryptionKeys:      []string{"v1:abcd"},
		AppEncryptionActiveKey: "v1",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected env encryption provider to validate in production, got %v", err)
	}
}

func TestValidateRejectsKMSStubInProduction(t *testing.T) {
	cfg := Config{
		AppEnv:                 "production",
		DatabaseURL:            "postgres://localhost:5435/paygate?sslmode=disable",
		DashboardSessionSecret: "abcdefghijklmnopqrstuvwxyz012345",
		PayoutRailSecret:       "abcdefghijklmnopqrstuvwxyz",
		TrustedProxyCIDRs:      []string{"127.0.0.1/32"},
		AppEncryptionProvider:  "kms_stub",
		AppEncryptionKMSKeyURI: "kms://stub/test",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for kms_stub provider in production")
	}
}
