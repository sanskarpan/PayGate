import { requireViewer, getPrometheusMetricSeries, getPrometheusMetricSum } from "../../lib/api";
import { formatCompactNumber } from "../../lib/types";

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
    gatewayAuthTotals,
    gatewayAuthLatencySum,
    gatewayAuthLatencyCount,
    webhookSignatureTotals,
  ] = await Promise.all([
    getPrometheusMetricSum("paygate_payments_total"),
    getPrometheusMetricSum("paygate_orders_total"),
    getPrometheusMetricSum("paygate_refunds_total"),
    getPrometheusMetricSum("paygate_webhook_deliveries_total"),
    getPrometheusMetricSum("paygate_outbox_unpublished_total"),
    getPrometheusMetricSum("paygate_disputes_total"),
    getPrometheusMetricSum("paygate_payouts_total"),
    getPrometheusMetricSum("paygate_http_requests_total"),
    getPrometheusMetricSeries("paygate_gateway_authorizations_total"),
    getPrometheusMetricSeries("paygate_gateway_authorization_duration_seconds_sum"),
    getPrometheusMetricSeries("paygate_gateway_authorization_duration_seconds_count"),
    getPrometheusMetricSeries("paygate_webhook_signature_deliveries_total"),
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

  const counters = [
    { label: "Payments", value: paymentsTotal ?? 0, note: "attempts" },
    { label: "Orders", value: ordersTotal ?? 0, note: "created" },
    { label: "Refunds", value: refundsTotal ?? 0, note: "issued" },
    { label: "Webhook Deliveries", value: webhookDeliveries ?? 0, note: "attempts" },
    { label: "Disputes", value: disputesTotal ?? 0, note: "cases" },
    { label: "Payouts", value: payoutsTotal ?? 0, note: "transfers" },
  ];
  const maxCounter = Math.max(...counters.map((item) => item.value), 1);

  const gatewayRows = (() => {
    const totalMap = new Map<string, { method: string; provider: string; authorized: number; declined: number; requiresAction: number; error: number }>();
    for (const point of gatewayAuthTotals) {
      const method = point.labels.method || "unknown";
      const provider = point.labels.provider || "unknown";
      const outcome = point.labels.outcome || "unknown";
      const key = `${method}:${provider}`;
      const current = totalMap.get(key) || { method, provider, authorized: 0, declined: 0, requiresAction: 0, error: 0 };
      if (outcome === "authorized") current.authorized += point.value;
      if (outcome === "declined") current.declined += point.value;
      if (outcome === "requires_action") current.requiresAction += point.value;
      if (outcome === "error") current.error += point.value;
      totalMap.set(key, current);
    }
    const latencySumMap = new Map<string, number>();
    const latencyCountMap = new Map<string, number>();
    for (const point of gatewayAuthLatencySum) {
      const method = point.labels.method || "unknown";
      const provider = point.labels.provider || "unknown";
      latencySumMap.set(`${method}:${provider}`, point.value);
    }
    for (const point of gatewayAuthLatencyCount) {
      const method = point.labels.method || "unknown";
      const provider = point.labels.provider || "unknown";
      latencyCountMap.set(`${method}:${provider}`, point.value);
    }
    return Array.from(totalMap.values()).map((row) => {
      const key = `${row.method}:${row.provider}`;
      const total = row.authorized + row.declined + row.requiresAction + row.error;
      const successRate = total > 0 ? ((row.authorized + row.requiresAction) / total) * 100 : 0;
      const avgLatencyMs = latencyCountMap.get(key)
        ? ((latencySumMap.get(key) || 0) / (latencyCountMap.get(key) || 1)) * 1000
        : 0;
      return { ...row, total, successRate, avgLatencyMs };
    });
  })();

  const webhookSignatureRows = (() => {
    const byMode = new Map<string, { mode: string; succeeded: number; failed: number; deadLettered: number }>();
    for (const point of webhookSignatureTotals) {
      const mode = point.labels.signature_mode || "unknown";
      const status = point.labels.status || "unknown";
      const current = byMode.get(mode) || { mode, succeeded: 0, failed: 0, deadLettered: 0 };
      if (status === "succeeded") current.succeeded += point.value;
      if (status === "failed") current.failed += point.value;
      if (status === "dead_lettered") current.deadLettered += point.value;
      byMode.set(mode, current);
    }
    return Array.from(byMode.values());
  })();

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card">
          <div className="eyebrow">System Health</div>
          <h1>Observability</h1>
          <p className="lede">
            Cumulative platform counters since last restart. Use this surface for rapid operator
            orientation, then move into Grafana for time-series and alert detail on{" "}
            <a href="http://localhost:3100" target="_blank" rel="noreferrer">
              localhost:3100
            </a>.
          </p>
          <div className="metric-strip">
            <div className="metric-chip">
              <span className="metric-chip-label">Outbox</span>
              <strong>{outboxUnpublished ?? 0}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Traffic</span>
              <strong>{formatCompactNumber(httpRequests)}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Webhook attempts</span>
              <strong>{formatCompactNumber(webhookDeliveries)}</strong>
            </div>
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Control Tower</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Queue Posture</span>
              <strong>{outboxOk ? "Healthy" : outboxUnpublished === null ? "Unknown" : "Attention"}</strong>
              <p>Backlog status for unpublished events and asynchronous downstream pressure.</p>
            </div>
            <div className="status-block">
              <span>Gateway Surface</span>
              <strong>{gatewayRows.length}</strong>
              <p>Method and provider combinations currently contributing authorization telemetry.</p>
            </div>
            <div className="status-block">
              <span>Signature Modes</span>
              <strong>{webhookSignatureRows.length}</strong>
              <p>Webhook signing profiles observed across live delivery traffic.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Delivery Health</span>
          <strong>{outboxOk ? "Healthy" : outboxUnpublished === null ? "Unknown" : "Needs attention"}</strong>
          <span>Outbox backlog is {outboxUnpublished ?? "unknown"}.</span>
        </div>
        <div className="ops-band-item">
          <span>Request Volume</span>
          <strong>{fmt(httpRequests)}</strong>
          <span>Aggregate API and dashboard traffic since process start.</span>
        </div>
        <div className="ops-band-item">
          <span>Commercial Load</span>
          <strong>{fmt(paymentsTotal)}</strong>
          <span>Payment attempts driving downstream lifecycle activity.</span>
        </div>
        <div className="ops-band-item">
          <span>Webhook Pressure</span>
          <strong>{fmt(webhookDeliveries)}</strong>
          <span>Observed delivery attempts across active subscriptions.</span>
        </div>
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
        <div className="section-head">
          <div>
            <h2>Counter distribution</h2>
            <div className="section-kicker">Relative scale across major platform counters</div>
          </div>
        </div>
        <div className="stack" style={{ marginTop: "16px" }}>
          {counters.map((item) => (
            <div className="signal-row" key={item.label}>
              <div className="signal-head">
                <div className="row-title">{item.label}</div>
                <div className="row-meta">
                  <span>{fmt(item.value)}</span>
                  <span>{item.note}</span>
                </div>
              </div>
              <div className="signal-track">
                <div
                  className="signal-fill"
                  style={{ width: `${Math.max(8, (item.value / maxCounter) * 100)}%` }}
                />
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="detail-card">
        <div className="section-head">
          <div>
            <h2>Telemetry brief</h2>
            <div className="section-kicker">Operator translation of the current machine posture</div>
          </div>
        </div>
        <div className="intel-grid" style={{ marginTop: "16px" }}>
          <div className="intel-card">
            <span>Async pipeline</span>
            <strong>{outboxUnpublished ?? 0} backlog</strong>
            <p>Use this as the first indicator of relay pressure, retry loops, or downstream publish stalls.</p>
          </div>
          <div className="intel-card">
            <span>Authorization mesh</span>
            <strong>{gatewayRows.length} lanes</strong>
            <p>Each lane represents a method-provider pair with distinct success and latency characteristics.</p>
          </div>
          <div className="intel-card">
            <span>Signature governance</span>
            <strong>{webhookSignatureRows.length} modes</strong>
            <p>Track which merchants still depend on compatibility mode versus stronger message-signature posture.</p>
          </div>
          <div className="intel-card">
            <span>Traffic base</span>
            <strong>{fmt(httpRequests)} requests</strong>
            <p>Interpret all current counters as process-lifetime cumulative telemetry, not time-window samples.</p>
          </div>
        </div>
      </div>

      <div className="detail-grid">
        <div className="list-card">
          <div className="section-head">
            <div>
              <h2>Method and provider SLOs</h2>
              <div className="section-kicker">Live authorization success and mean latency by payment method and gateway provider</div>
            </div>
          </div>
          {gatewayRows.length === 0 ? (
            <div className="empty-state">
              <strong>No gateway SLO samples yet</strong>
              <span className="muted">Run payment traffic to populate method and provider breakdowns.</span>
            </div>
          ) : (
            gatewayRows.map((row) => (
              <div className="list-row" key={`${row.method}:${row.provider}`}>
                <div>
                  <div className="row-title">{row.method} · {row.provider}</div>
                  <div className="row-meta">
                    <span>{row.total.toFixed(0)} auth attempts</span>
                    <span>{row.authorized.toFixed(0)} authorized</span>
                    <span>{row.requiresAction.toFixed(0)} challenge/async</span>
                    <span>{row.declined.toFixed(0)} declined</span>
                    <span>{row.error.toFixed(0)} errors</span>
                  </div>
                </div>
                <div className="row-actions">
                  <div className="amount-pill">{row.successRate.toFixed(1)}% success</div>
                  <div className="amount-pill">{row.avgLatencyMs.toFixed(0)}ms avg</div>
                </div>
              </div>
            ))
          )}
        </div>

        <div className="list-card">
          <div className="section-head">
            <div>
              <h2>Webhook signing posture</h2>
              <div className="section-kicker">Delivery behavior by signature mode</div>
            </div>
          </div>
          {webhookSignatureRows.length === 0 ? (
            <div className="empty-state">
              <strong>No webhook deliveries yet</strong>
              <span className="muted">Deliver events to see signature-mode adoption and failure posture.</span>
            </div>
          ) : (
            webhookSignatureRows.map((row) => (
              <div className="list-row" key={row.mode}>
                <div>
                  <div className="row-title">{row.mode}</div>
                  <div className="row-meta">
                    <span>{row.succeeded.toFixed(0)} succeeded</span>
                    <span>{row.failed.toFixed(0)} failed</span>
                    <span>{row.deadLettered.toFixed(0)} dead-lettered</span>
                  </div>
                </div>
              </div>
            ))
          )}
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
        <h2>Monitoring Stack</h2>
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
