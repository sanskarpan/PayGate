import Link from "next/link";

import {
  getDisputes,
  getOrders,
  getPayouts,
  getPrometheusMetricSum,
  getRiskEvents,
  getSettlements,
  getWebhooks,
  requireViewer,
} from "../../lib/api";
import { formatCompactNumber, formatMoney, formatTime, truncateMiddle } from "../../lib/types";

export default async function OverviewPage() {
  const viewer = await requireViewer();
  const [
    orders,
    disputes,
    risk,
    webhooks,
    settlements,
    payouts,
    paymentsTotal,
    httpRequests,
    outboxUnpublished,
  ] = await Promise.all([
    getOrders(),
    getDisputes(),
    getRiskEvents(),
    getWebhooks(),
    getSettlements(),
    getPayouts(),
    getPrometheusMetricSum("paygate_payments_total"),
    getPrometheusMetricSum("paygate_http_requests_total"),
    getPrometheusMetricSum("paygate_outbox_unpublished_total"),
  ]);

  const openDisputes = disputes.items.filter((item) =>
    item.status === "open" || item.status === "under_review"
  ).length;
  const unresolvedRisk = risk.items.filter((item) => !item.resolved).length;
  const activeWebhooks = webhooks.items.filter((item) => item.status === "active").length;
  const pendingPayouts = payouts.items.filter((item) => item.status !== "completed").length;
  const latestSettlement = settlements.items[0];

  return (
    <section className="stack fade-up">
      <div className="hero-card">
        <div className="eyebrow">Operations Snapshot</div>
        <h1>Merchant command center.</h1>
        <p className="lede">
          Track money flow, operator risk, asynchronous delivery health, and settlement posture
          for <strong> {viewer.merchant_id}</strong>.
        </p>
        <div className="metric-strip">
          <div className="metric-chip">
            <span className="metric-chip-label">Payments</span>
            <strong>{formatCompactNumber(paymentsTotal)}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Requests</span>
            <strong>{formatCompactNumber(httpRequests)}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Outbox</span>
            <strong>{outboxUnpublished ?? 0}</strong>
          </div>
        </div>
        <div className="hero-actions">
          <Link className="primary-button" href="/orders">
            Review Orders
          </Link>
          <Link className="ghost-button" href="/observability">
            Open Observability
          </Link>
          <Link className="ghost-button" href="/disputes">
            Inspect Disputes
          </Link>
          <Link className="ghost-button" href="/compliance">
            Review Compliance
          </Link>
        </div>
      </div>

      <div className="summary-grid">
        <div className="metric-card">
          <div className="eyebrow">Orders</div>
          <div className="stat-value">{orders.count}</div>
          <div className="stat-label">visible records in the current dashboard scope</div>
        </div>
        <div className="metric-card">
          <div className="eyebrow">Active Webhooks</div>
          <div className="stat-value">{activeWebhooks}</div>
          <div className="stat-label">subscriptions ready to receive events</div>
        </div>
        <div className="metric-card">
          <div className="eyebrow">Open Disputes</div>
          <div className="stat-value">{openDisputes}</div>
          <div className="stat-label">cases needing evidence or review decisions</div>
        </div>
        <div className="metric-card">
          <div className="eyebrow">Pending Payouts</div>
          <div className="stat-value">{pendingPayouts}</div>
          <div className="stat-label">transfers not yet fully completed</div>
        </div>
      </div>

      <div className="rail-grid">
        <div className="stack">
          <div className="detail-card">
            <div className="section-head">
              <div>
                <div className="eyebrow">Watchlist</div>
                <h2>Operator attention</h2>
              </div>
              <Link className="ghost-button" href="/risk">
                Risk Queue
              </Link>
            </div>
            <div className="summary-grid" style={{ marginTop: "16px" }}>
              <div className="metric-card">
                <div className="eyebrow">Risk</div>
                <div className="stat-value">{unresolvedRisk}</div>
                <div className="stat-label">unresolved risk events</div>
              </div>
              <div className="metric-card">
                <div className="eyebrow">Disputes</div>
                <div className="stat-value">{openDisputes}</div>
                <div className="stat-label">open or under review</div>
              </div>
              <div className="metric-card">
                <div className="eyebrow">Async Health</div>
                <div className="stat-value">{outboxUnpublished ?? 0}</div>
                <div className="stat-label">events waiting for publish</div>
              </div>
              <div className="metric-card">
                <div className="eyebrow">Settlement</div>
                <div className="stat-value">
                  {latestSettlement ? formatMoney(latestSettlement.net_amount, latestSettlement.currency) : "—"}
                </div>
                <div className="stat-label">latest net batch amount</div>
              </div>
            </div>
          </div>

          <div className="detail-card">
            <div className="section-head">
              <div>
                <div className="eyebrow">Recent Orders</div>
                <h2>Newest checkout activity</h2>
              </div>
              <Link className="ghost-button" href="/orders">
                Full Order List
              </Link>
            </div>
            <div className="stack" style={{ marginTop: "16px" }}>
              {orders.items.length === 0 ? (
                <div className="empty-state">
                  <strong>No orders yet</strong>
                  <span className="muted">New payment sessions will start showing here as they are created.</span>
                </div>
              ) : (
                orders.items.slice(0, 5).map((order) => (
                  <Link className="list-row" href={`/orders/${order.id}`} key={order.id}>
                    <div className="entity-primary">
                      <div className="row-title">{truncateMiddle(order.id)}</div>
                      <div className="row-meta">
                        <span className="badge-neutral">{order.status}</span>
                        <span>{order.receipt || "Receipt pending"}</span>
                        <span>{formatTime(order.created_at)}</span>
                      </div>
                    </div>
                    <div className="amount-pill">{formatMoney(order.amount, order.currency)}</div>
                  </Link>
                ))
              )}
            </div>
          </div>
        </div>

        <div className="stack">
          <div className="spotlight-card">
            <div className="eyebrow">Settlement Posture</div>
            <h2>Latest batch</h2>
            {latestSettlement ? (
              <>
                <div className="stat-value">
                  {formatMoney(latestSettlement.net_amount, latestSettlement.currency)}
                </div>
                <div className="row-meta">
                  <span className="badge-success">{latestSettlement.status}</span>
                  <span>{latestSettlement.payment_count} payments</span>
                </div>
                <p className="muted">
                  Period {formatTime(latestSettlement.period_start)} to {formatTime(latestSettlement.period_end)}.
                </p>
                <Link className="ghost-button" href={`/settlements/${latestSettlement.id}`}>
                  View Batch Detail
                </Link>
              </>
            ) : (
              <div className="empty-state">
                <strong>No settlement batches</strong>
                <span className="muted">Run a settlement cycle to surface merchant payout readiness.</span>
              </div>
            )}
          </div>

          <div className="detail-card">
            <div className="eyebrow">Control Rails</div>
            <h2>High-leverage actions</h2>
            <div className="command-list" style={{ marginTop: "16px" }}>
              <Link className="list-row" href="/api-keys">
                <div>
                  <div className="row-title">Integration credentials</div>
                  <div className="row-meta">
                    <span>Issue, revoke, and restrict API keys</span>
                  </div>
                </div>
              </Link>
              <Link className="list-row" href="/webhooks">
                <div>
                  <div className="row-title">Webhook reliability</div>
                  <div className="row-meta">
                    <span>Inspect subscriptions, retries, and delivery outcomes</span>
                  </div>
                </div>
              </Link>
              <Link className="list-row" href="/gateway">
                <div>
                  <div className="row-title">Failure-path simulation</div>
                  <div className="row-meta">
                    <span>Control gateway behavior for testing adverse scenarios</span>
                  </div>
                </div>
              </Link>
              <Link className="list-row" href="/reports">
                <div>
                  <div className="row-title">Finance downloads</div>
                  <div className="row-meta">
                    <span>Generate statements, exports, and tax reporting artifacts</span>
                  </div>
                </div>
              </Link>
              <Link className="list-row" href="/control-plane">
                <div>
                  <div className="row-title">Runtime control plane</div>
                  <div className="row-meta">
                    <span>Inspect sagas, schemas, and ledger holds</span>
                  </div>
                </div>
              </Link>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
