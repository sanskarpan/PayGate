import { requireViewer, getPrometheusMetricSum } from "../../lib/api";

export default async function ObservabilityPage() {
  await requireViewer();

  const [
    paymentsTotal,
    ordersTotal,
    refundsTotal,
    webhookDeliveries,
    outboxUnpublished,
    disputesTotal,
    payoutsTotal,
    httpRequests,
  ] = await Promise.all([
    getPrometheusMetricSum("paygate_payments_total"),
    getPrometheusMetricSum("paygate_orders_total"),
    getPrometheusMetricSum("paygate_refunds_total"),
    getPrometheusMetricSum("paygate_webhook_deliveries_total"),
    getPrometheusMetricSum("paygate_outbox_unpublished_total"),
    getPrometheusMetricSum("paygate_disputes_total"),
    getPrometheusMetricSum("paygate_payouts_total"),
    getPrometheusMetricSum("paygate_http_requests_total"),
  ]);

  function fmt(n: number | null): string {
    if (n === null) return "—";
    if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + "M";
    if (n >= 1_000) return (n / 1_000).toFixed(1) + "K";
    return n.toFixed(0);
  }

  const outboxOk = outboxUnpublished !== null && outboxUnpublished < 100;
  const outboxBadge = outboxUnpublished === null
    ? "badge-neutral"
    : outboxUnpublished >= 500
    ? "badge-error"
    : outboxUnpublished >= 100
    ? "badge-warning"
    : "badge-success";

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">System Health</div>
        <h1>Observability</h1>
        <p className="lede">
          Cumulative metrics since last restart. For time-series charts and
          alerting, open Grafana on{" "}
          <a href="http://localhost:3100" target="_blank" rel="noreferrer">
            localhost:3100
          </a>{" "}
          (admin / paygate).
        </p>
      </div>

      <div className="detail-grid">
        <div className="metric-card">
          <div className="eyebrow">Payments</div>
          <div style={{ fontSize: "2.4rem", fontWeight: 700, margin: "6px 0 2px" }}>
            {fmt(paymentsTotal)}
          </div>
          <div className="muted" style={{ fontSize: "0.9rem" }}>total attempts (all statuses)</div>
        </div>
        <div className="metric-card">
          <div className="eyebrow">Orders</div>
          <div style={{ fontSize: "2.4rem", fontWeight: 700, margin: "6px 0 2px" }}>
            {fmt(ordersTotal)}
          </div>
          <div className="muted" style={{ fontSize: "0.9rem" }}>total orders created</div>
        </div>
        <div className="metric-card">
          <div className="eyebrow">Refunds</div>
          <div style={{ fontSize: "2.4rem", fontWeight: 700, margin: "6px 0 2px" }}>
            {fmt(refundsTotal)}
          </div>
          <div className="muted" style={{ fontSize: "0.9rem" }}>total refunds (all statuses)</div>
        </div>
        <div className="metric-card">
          <div className="eyebrow">Webhook Deliveries</div>
          <div style={{ fontSize: "2.4rem", fontWeight: 700, margin: "6px 0 2px" }}>
            {fmt(webhookDeliveries)}
          </div>
          <div className="muted" style={{ fontSize: "0.9rem" }}>total delivery attempts</div>
        </div>
        <div className="metric-card">
          <div className="eyebrow">Disputes</div>
          <div style={{ fontSize: "2.4rem", fontWeight: 700, margin: "6px 0 2px" }}>
            {fmt(disputesTotal)}
          </div>
          <div className="muted" style={{ fontSize: "0.9rem" }}>total chargebacks</div>
        </div>
        <div className="metric-card">
          <div className="eyebrow">Payouts</div>
          <div style={{ fontSize: "2.4rem", fontWeight: 700, margin: "6px 0 2px" }}>
            {fmt(payoutsTotal)}
          </div>
          <div className="muted" style={{ fontSize: "0.9rem" }}>total bank transfers initiated</div>
        </div>
        <div className="metric-card">
          <div className="eyebrow">HTTP Requests</div>
          <div style={{ fontSize: "2.4rem", fontWeight: 700, margin: "6px 0 2px" }}>
            {fmt(httpRequests)}
          </div>
          <div className="muted" style={{ fontSize: "0.9rem" }}>total API requests served</div>
        </div>
        <div className="metric-card" style={{ position: "relative" }}>
          <div className="eyebrow">Outbox Backlog</div>
          <div style={{ fontSize: "2.4rem", fontWeight: 700, margin: "6px 0 2px" }}>
            {fmt(outboxUnpublished)}
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
            <span className={outboxBadge} style={{ fontSize: "0.75rem" }}>
              {outboxOk ? "healthy" : outboxUnpublished === null ? "unknown" : "backlogged"}
            </span>
            <span className="muted" style={{ fontSize: "0.9rem" }}>unpublished events</span>
          </div>
        </div>
      </div>

      <div className="detail-card">
        <h2>Grafana Dashboards</h2>
        <p className="muted" style={{ marginBottom: "1rem" }}>
          These pre-built dashboards are provisioned automatically when you start the monitoring stack.
        </p>
        <div className="stack">
          {[
            {
              name: "Payment Funnel",
              desc: "Authorization rate, capture rate, failure rate, p99 latency",
              uid: "paygate-payment-funnel",
            },
            {
              name: "Webhook Delivery",
              desc: "Delivery success rate, dead-letter queue, outbox backlog",
              uid: "paygate-webhook-delivery",
            },
            {
              name: "Settlement & Payouts",
              desc: "Settlement volume, payout status, refund vs capture rate",
              uid: "paygate-settlement",
            },
          ].map(({ name, desc, uid }) => (
            <div className="list-row" key={uid}>
              <div>
                <div className="row-title">{name}</div>
                <div className="row-meta">
                  <span>{desc}</span>
                </div>
              </div>
              <a
                href={`http://localhost:3100/d/${uid}`}
                target="_blank"
                rel="noreferrer"
                className="action-button"
              >
                Open →
              </a>
            </div>
          ))}
        </div>
      </div>

      <div className="detail-card">
        <h2>Start Monitoring Stack</h2>
        <p className="muted">
          Prometheus, Grafana, and Alertmanager are defined in <code>docker-compose.yml</code>.
        </p>
        <pre style={{ background: "var(--surface)", padding: "1rem", borderRadius: "0.5rem", overflow: "auto", fontSize: "0.85rem", marginTop: "0.75rem" }}>
{`# Start monitoring stack
docker compose up prometheus grafana alertmanager -d

# Prometheus:     http://localhost:9090
# Grafana:        http://localhost:3100  (admin / paygate)
# Alertmanager:   http://localhost:9093`}
        </pre>
      </div>
    </section>
  );
}
