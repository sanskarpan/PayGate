package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Payment metrics
	PaymentsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "paygate_payments_total",
		Help: "Total number of payment attempts",
	}, []string{"merchant_id", "status"}) // status: authorized, captured, failed

	PaymentAmountTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "paygate_payment_amount_paise_total",
		Help: "Total payment amount in paise",
	}, []string{"merchant_id", "currency"})

	PaymentDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "paygate_payment_duration_seconds",
		Help:    "Payment authorization duration in seconds",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
	}, []string{"merchant_id"})

	// Order metrics
	OrdersTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "paygate_orders_total",
		Help: "Total number of orders created",
	}, []string{"merchant_id", "currency"})

	// Refund metrics
	RefundsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "paygate_refunds_total",
		Help: "Total number of refunds",
	}, []string{"merchant_id", "status"})

	RefundAmountTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "paygate_refund_amount_paise_total",
		Help: "Total refund amount in paise",
	}, []string{"merchant_id", "currency"})

	// Settlement metrics
	SettlementsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "paygate_settlements_total",
		Help: "Total settlement batches created",
	}, []string{"merchant_id"})

	SettlementNetAmount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "paygate_settlement_net_amount_paise_total",
		Help: "Total net settlement amount in paise",
	}, []string{"merchant_id", "currency"})

	// Webhook metrics
	WebhookDeliveriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "paygate_webhook_deliveries_total",
		Help: "Total webhook delivery attempts",
	}, []string{"status"}) // status: success, failed, dead_lettered

	// Risk metrics
	RiskEventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "paygate_risk_events_total",
		Help: "Total risk evaluation events",
	}, []string{"merchant_id", "action"}) // action: allow, hold, block

	// Dispute metrics
	DisputesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "paygate_disputes_total",
		Help: "Total disputes created",
	}, []string{"merchant_id", "reason"})

	DisputesResolved = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "paygate_disputes_resolved_total",
		Help: "Total disputes resolved",
	}, []string{"merchant_id", "outcome"}) // outcome: won, lost, accepted

	// Payout metrics
	PayoutsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "paygate_payouts_total",
		Help: "Total payouts initiated",
	}, []string{"merchant_id", "status"})

	// HTTP metrics
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "paygate_http_requests_total",
		Help: "Total HTTP requests",
	}, []string{"method", "path", "status_code"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "paygate_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	// Outbox metrics
	OutboxUnpublished = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "paygate_outbox_unpublished_total",
		Help: "Number of outbox entries not yet published to Kafka",
	})

	EventSchemaPublishesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "paygate_event_schema_publishes_total",
		Help: "Total published event envelopes by schema subject and version",
	}, []string{"topic", "event_type", "schema_subject", "schema_version"})
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }

// MetricsMiddleware records HTTP request counts and durations.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		duration := time.Since(start).Seconds()
		path := r.URL.Path
		// Normalize path to avoid high cardinality (strip IDs)
		// Simple approach: just use the pattern prefix
		HTTPRequestsTotal.WithLabelValues(r.Method, path, fmt.Sprintf("%d", rec.status)).Inc()
		HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	})
}
