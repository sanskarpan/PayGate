package config

import (
	"os"
	"strings"
)

type Config struct {
	Port                   string
	DatabaseURL            string
	RedisAddr              string
	KafkaBrokers           []string
	DashboardSessionSecret string
}

func FromEnv() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// #nosec G101 -- local development default DSN
		dbURL = "postgres://paygate:paygate@localhost:5432/paygate?sslmode=disable"
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
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
	return Config{
		Port:                   port,
		DatabaseURL:            dbURL,
		RedisAddr:              redisAddr,
		KafkaBrokers:           splitCSV(kafkaBrokers),
		DashboardSessionSecret: sessionSecret,
	}
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
