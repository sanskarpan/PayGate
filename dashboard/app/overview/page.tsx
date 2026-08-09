import Link from "next/link";

import { OrbitalRig, WireTunnel } from "../../components/system-icons";
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
  const latestOrder = orders.items[0];

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
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
          <div className="surface-visual-shell surface-visual-shell-hero" aria-hidden="true">
            <WireTunnel className="surface-wireframe" size={320} />
            <OrbitalRig className="surface-render surface-render-overview" />
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Immediate Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Async Queue</span>
              <strong>{outboxUnpublished ?? 0}</strong>
              <p>Events waiting for publish across the transactional outbox.</p>
            </div>
            <div className="status-block">
              <span>Risk Pressure</span>
              <strong>{unresolvedRisk}</strong>
              <p>Unresolved risk events requiring operator or policy attention.</p>
            </div>
            <div className="status-block">
              <span>Latest Order</span>
              <strong>{latestOrder ? truncateMiddle(latestOrder.id) : "No traffic"}</strong>
              <p>{latestOrder ? `${latestOrder.status} · ${formatTime(latestOrder.created_at)}` : "Awaiting new payment sessions."}</p>
            </div>
          </div>
          <div className="command-deck">
            <Link className="command-link" href="/control-plane">
              <strong>Runtime control plane</strong>
              <span>Inspect sagas, ledger holds, and schema governance surfaces.</span>
            </Link>
            <Link className="command-link" href="/gateway">
              <strong>Failure simulation rail</strong>
              <span>Drive gateway conditions to probe adverse-path behavior.</span>
            </Link>
            <Link className="command-link" href="/reports">
              <strong>Finance export lane</strong>
              <span>Generate statements, payout artifacts, and tax-oriented data slices.</span>
            </Link>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Orders in Scope</span>
          <strong>{orders.count}</strong>
          <span>Visible records across the current operator view.</span>
        </div>
        <div className="ops-band-item">
          <span>Active Webhooks</span>
          <strong>{activeWebhooks}</strong>
          <span>Subscriptions ready to receive merchant events.</span>
        </div>
        <div className="ops-band-item">
          <span>Open Disputes</span>
          <strong>{openDisputes}</strong>
          <span>Cases requiring evidence or review decisions.</span>
        </div>
        <div className="ops-band-item">
          <span>Pending Payouts</span>
          <strong>{pendingPayouts}</strong>
          <span>Transfers still moving through treasury lanes.</span>
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
            <div className="intel-grid" style={{ marginTop: "16px" }}>
              <div className="intel-card">
                <span>Risk backlog</span>
                <strong>{unresolvedRisk}</strong>
                <p>Unresolved risk events that can affect capture, reserve posture, or manual review.</p>
              </div>
              <div className="intel-card">
                <span>Dispute heat</span>
                <strong>{openDisputes}</strong>
                <p>Open or under-review disputes that can alter merchant loss posture.</p>
              </div>
              <div className="intel-card">
                <span>Async health</span>
                <strong>{outboxUnpublished ?? 0}</strong>
                <p>Events waiting for publish across webhook and downstream processing lanes.</p>
              </div>
              <div className="intel-card">
                <span>Settlement posture</span>
                <strong>{latestSettlement ? formatMoney(latestSettlement.net_amount, latestSettlement.currency) : "—"}</strong>
                <p>Latest net batch amount available to treasury and payout workflows.</p>
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
            <div className="command-deck" style={{ marginTop: "16px" }}>
              <Link className="command-link" href="/api-keys">
                <strong>Integration credentials</strong>
                <span>Issue, revoke, and restrict API keys at merchant boundary.</span>
              </Link>
              <Link className="command-link" href="/webhooks">
                <strong>Webhook reliability</strong>
                <span>Inspect subscriptions, retry exhaustion, and delivery outcomes.</span>
              </Link>
              <Link className="command-link" href="/gateway">
                <strong>Failure-path simulation</strong>
                <span>Drive gateway scenarios to test adverse operator conditions.</span>
              </Link>
              <Link className="command-link" href="/reports">
                <strong>Finance downloads</strong>
                <span>Generate statements, exports, and tax reporting artifacts.</span>
              </Link>
              <Link className="command-link" href="/control-plane">
                <strong>Runtime control plane</strong>
                <span>Inspect sagas, schemas, and ledger holds.</span>
              </Link>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
