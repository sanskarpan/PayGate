import Link from "next/link";

import { getWebhooks, requireViewer } from "../../lib/api";
import { formatCompactNumber, formatTime, truncateMiddle } from "../../lib/types";

export default async function WebhooksPage() {
  const viewer = await requireViewer();
  const webhooks = await getWebhooks();
  const activeCount = webhooks.items.filter((item) => item.status === "active").length;

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card">
          <div className="eyebrow">Async Delivery</div>
          <h1>Webhooks</h1>
          <p className="lede">
            {webhooks.count} subscription{webhooks.count !== 1 ? "s" : ""} configured for{" "}
            {viewer.merchant_id}.
          </p>
          <div className="metric-strip">
            <div className="metric-chip">
              <span className="metric-chip-label">Subscriptions</span>
              <strong>{formatCompactNumber(webhooks.count)}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Active</span>
              <strong>{activeCount}</strong>
            </div>
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Delivery Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Active subscriptions</span>
              <strong>{activeCount}</strong>
              <p>Merchant endpoints currently eligible to receive live event dispatches.</p>
            </div>
            <div className="status-block">
              <span>Inactive subscriptions</span>
              <strong>{webhooks.count - activeCount}</strong>
              <p>Configured endpoints not currently in fully active delivery posture.</p>
            </div>
            <div className="status-block">
              <span>Event footprint</span>
              <strong>{webhooks.items.reduce((acc, item) => acc + item.events.length, 0)}</strong>
              <p>Total event-topic bindings currently spread across the subscription inventory.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Subscriptions</span>
          <strong>{webhooks.count}</strong>
          <span>Total merchant webhook registrations currently present.</span>
        </div>
        <div className="ops-band-item">
          <span>Active</span>
          <strong>{activeCount}</strong>
          <span>Registrations currently able to receive delivery traffic.</span>
        </div>
        <div className="ops-band-item">
          <span>Inactive</span>
          <strong>{webhooks.count - activeCount}</strong>
          <span>Configured endpoints outside fully active delivery state.</span>
        </div>
        <div className="ops-band-item">
          <span>Merchant scope</span>
          <strong>{truncateMiddle(viewer.merchant_id)}</strong>
          <span>Current webhook operating boundary.</span>
        </div>
      </div>

      <div className="list-card">
        <div className="section-head">
          <div>
            <h2>Endpoints</h2>
            <div className="section-kicker">Subscription health, event scope, and registration time</div>
          </div>
        </div>
        {webhooks.items.length === 0 ? (
          <div className="empty-state">
            <strong>No webhook subscriptions</strong>
            <span className="muted">Add merchant endpoints to start receiving payment lifecycle events.</span>
          </div>
        ) : (
          webhooks.items.map((wh) => (
            <Link className="list-row" href={`/webhooks/${wh.id}`} key={wh.id}>
              <div className="entity-primary">
                <div className="row-title">{wh.url}</div>
                <div className="mono">{truncateMiddle(wh.id)}</div>
                <div className="row-meta">
                  <span
                    className={
                      wh.status === "active" ? "badge-success" : "badge-warning"
                    }
                  >
                    {wh.status}
                  </span>
                  {wh.events.map((event) => (
                    <span className="badge-info" key={event}>{event}</span>
                  ))}
                  <span>{formatTime(wh.created_at)}</span>
                </div>
              </div>
            </Link>
          ))
        )}
      </div>
    </section>
  );
}
