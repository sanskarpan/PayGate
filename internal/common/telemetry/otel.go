package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// Init sets up the OpenTelemetry trace pipeline.
//
// When OTEL_EXPORTER_OTLP_ENDPOINT is set, spans are sent to that collector
// endpoint via OTLP/HTTP. Stdout tracing is only enabled when
// OTEL_EXPORTER_STDOUT=true. Otherwise tracing is effectively disabled with a
// never-sampled provider so normal runtime logs stay readable.
func Init(ctx context.Context, service string) (func(context.Context) error, error) {
	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(service)))
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	mode := exporterModeFromEnv()
	if mode == telemetryModeDisabled {
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.NeverSample()),
			sdktrace.WithResource(res),
		)
		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(propagation.TraceContext{})
		return tp.Shutdown, nil
	}

	var exporter sdktrace.SpanExporter
	switch mode {
	case telemetryModeOTLP:
		endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
		exporter, err = otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(endpoint),
			otlptracehttp.WithInsecure(),
		)
		if err != nil {
			return nil, fmt.Errorf("create otlp otel exporter: %w", err)
		}
	case telemetryModeStdout:
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("create stdout otel exporter: %w", err)
		}
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return tp.Shutdown, nil
}

func WrapHTTP(handler http.Handler, operation string) http.Handler {
	return otelhttp.NewHandler(handler, operation)
}

type telemetryMode string

const (
	telemetryModeDisabled telemetryMode = "disabled"
	telemetryModeOTLP     telemetryMode = "otlp"
	telemetryModeStdout   telemetryMode = "stdout"
)

func exporterModeFromEnv() telemetryMode {
	if strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != "" {
		return telemetryModeOTLP
	}
	if isTruthy(os.Getenv("OTEL_EXPORTER_STDOUT")) {
		return telemetryModeStdout
	}
	return telemetryModeDisabled
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
