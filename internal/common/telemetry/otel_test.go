package telemetry

import (
	"testing"
)

func TestExporterModeFromEnv(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_STDOUT", "")
	if got := exporterModeFromEnv(); got != telemetryModeDisabled {
		t.Fatalf("expected disabled mode by default, got %q", got)
	}

	t.Setenv("OTEL_EXPORTER_STDOUT", "true")
	if got := exporterModeFromEnv(); got != telemetryModeStdout {
		t.Fatalf("expected stdout mode when explicitly enabled, got %q", got)
	}

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "collector.internal:4318")
	if got := exporterModeFromEnv(); got != telemetryModeOTLP {
		t.Fatalf("expected OTLP mode to override stdout, got %q", got)
	}
}
