package payment

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/sanskarpan/PayGate/internal/common/metrics"
)

type fakeRoutedGateway struct {
	result   GatewayAuthResult
	decision GatewayRouteDecision
	err      error
}

func (f fakeRoutedGateway) Name() string {
	return "router"
}

func (f fakeRoutedGateway) Authorize(context.Context, int64, string, string, string) (GatewayAuthResult, error) {
	return f.result, f.err
}

func (f fakeRoutedGateway) AuthorizeWithRouting(context.Context, int64, string, string, string) (GatewayAuthResult, GatewayRouteDecision, error) {
	return f.result, f.decision, f.err
}

func TestAuthorizeUsesEffectiveRoutedProviderForMetrics(t *testing.T) {
	metrics.GatewayAuthorizationsTotal.Reset()
	metrics.GatewayAuthorizationDuration.Reset()

	repo := &fakeRepo{}
	svc := NewService(repo, fakeRoutedGateway{
		result: GatewayAuthResult{
			Success:          true,
			GatewayReference: "gw_ref_1",
			AuthCode:         "auth_1",
		},
		decision: GatewayRouteDecision{
			Provider:           "fallback_provider",
			RoutingReason:      "fallback_after_primary_error",
			AttemptedProviders: []string{"primary_provider", "fallback_provider"},
			PrimaryProvider:    "primary_provider",
			FallbackUsed:       true,
		},
	})

	_, err := svc.Authorize(context.Background(), AuthorizeInput{
		PaymentID:      "pay_1",
		MerchantID:     "merch_1",
		OrderID:        "ord_1",
		Amount:         1000,
		Currency:       "INR",
		Method:         "card",
		IdempotencyKey: "idem_1",
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}

	if got := testutil.ToFloat64(metrics.GatewayAuthorizationsTotal.WithLabelValues("card", "fallback_provider", "authorized")); got != 1 {
		t.Fatalf("expected authorized metric for fallback provider, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.GatewayAuthorizationsTotal.WithLabelValues("card", "router", "authorized")); got != 0 {
		t.Fatalf("expected no authorized metric for wrapper provider, got %v", got)
	}

	if count := histogramSampleCount(t, "card", "fallback_provider"); count != 1 {
		t.Fatalf("expected one duration sample for fallback provider, got %d", count)
	}
	if count := histogramSampleCount(t, "card", "router"); count != 0 {
		t.Fatalf("expected no duration samples for wrapper provider, got %d", count)
	}
}

func histogramSampleCount(t *testing.T, method, provider string) uint64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather histogram metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "paygate_gateway_authorization_duration_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			if labelValue(metric, "method") == method && labelValue(metric, "provider") == provider {
				return metric.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func labelValue(metric *dto.Metric, name string) string {
	for _, label := range metric.GetLabel() {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}
