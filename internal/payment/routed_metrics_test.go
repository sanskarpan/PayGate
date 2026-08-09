package payment

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/sanskarpan/PayGate/internal/common/metrics"
)

// counterValue reads a counter without pulling in the prometheus testutil
// package, which is not a dependency of this module.
func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	return m.GetCounter().GetValue()
}

// routedFakeGateway reports one name but routes the attempt to another
// provider, which is what failover looks like in production.
type routedFakeGateway struct {
	name     string
	routedTo string
}

func (g routedFakeGateway) Name() string { return g.name }

func (g routedFakeGateway) Authorize(context.Context, int64, string, string, string) (GatewayAuthResult, error) {
	return GatewayAuthResult{Success: true, GatewayReference: "ref", AuthCode: "auth"}, nil
}

func (g routedFakeGateway) AuthorizeWithRouting(context.Context, int64, string, string, string) (GatewayAuthResult, GatewayRouteDecision, error) {
	return GatewayAuthResult{Success: true, GatewayReference: "ref", AuthCode: "auth"},
		GatewayRouteDecision{
			Provider:           g.routedTo,
			RoutingReason:      "primary_failed",
			AttemptedProviders: []string{g.name, g.routedTo},
			PrimaryProvider:    g.name,
			FallbackUsed:       true,
		}, nil
}

// A fallback authorization must be counted against the provider that actually
// handled it. Attributing it to the wrapper gateway points incident analysis at
// the wrong provider and makes per-provider dashboards lie during an outage.
func TestAuthorizeLabelsMetricsWithEffectiveRoutedProvider(t *testing.T) {
	const method = "card"
	const wrapper = "router_wrapper_test"
	const routedTo = "fallback_provider_test"

	metrics.GatewayAuthorizationsTotal.Reset()
	metrics.GatewayAuthorizationDuration.Reset()

	svc := NewService(&fakeRepo{}, routedFakeGateway{name: wrapper, routedTo: routedTo})
	if _, err := svc.Authorize(context.Background(), AuthorizeInput{
		PaymentID:  "pay_routed_metrics",
		MerchantID: "merch_routed_metrics",
		OrderID:    "order_routed_metrics",
		Amount:     1000,
		Currency:   "INR",
		Method:     method,
	}); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	if got := counterValue(t, metrics.GatewayAuthorizationsTotal.WithLabelValues(method, routedTo, "authorized")); got != 1 {
		t.Fatalf("expected the authorization counted against the routed provider %q, got %v", routedTo, got)
	}
	if got := counterValue(t, metrics.GatewayAuthorizationsTotal.WithLabelValues(method, wrapper, "authorized")); got != 0 {
		t.Fatalf("expected nothing counted against the wrapper %q, got %v", wrapper, got)
	}
	var hist dto.Metric
	if err := metrics.GatewayAuthorizationDuration.WithLabelValues(method, routedTo).(prometheus.Metric).Write(&hist); err != nil {
		t.Fatalf("read histogram: %v", err)
	}
	if hist.GetHistogram().GetSampleCount() != 1 {
		t.Fatalf("expected one duration observation under %q, got %d", routedTo, hist.GetHistogram().GetSampleCount())
	}
}

// directFakeGateway does not implement RoutedGateway, so nothing routes.
type directFakeGateway struct{ name string }

func (g directFakeGateway) Name() string { return g.name }

func (g directFakeGateway) Authorize(context.Context, int64, string, string, string) (GatewayAuthResult, error) {
	return GatewayAuthResult{Success: true, GatewayReference: "ref", AuthCode: "auth"}, nil
}

// Direct gateways must keep emitting metrics under their own name.
func TestAuthorizeLabelsMetricsWithDirectProviderWhenNotRouted(t *testing.T) {
	const method = "card"
	const direct = "direct_provider_test"

	metrics.GatewayAuthorizationsTotal.Reset()

	svc := NewService(&fakeRepo{}, directFakeGateway{name: direct})
	if _, err := svc.Authorize(context.Background(), AuthorizeInput{
		PaymentID:  "pay_direct_metrics",
		MerchantID: "merch_direct_metrics",
		OrderID:    "order_direct_metrics",
		Amount:     1000,
		Currency:   "INR",
		Method:     method,
	}); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	if got := counterValue(t, metrics.GatewayAuthorizationsTotal.WithLabelValues(method, direct, "authorized")); got != 1 {
		t.Fatalf("expected the direct provider %q to be counted, got %v", direct, got)
	}
}
