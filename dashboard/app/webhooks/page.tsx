import Link from "next/link";

import { getWebhooks, requireViewer } from "../../lib/api";
import { formatCompactNumber, formatTime, truncateMiddle } from "../../lib/types";

export default async function WebhooksPage() {
  const viewer = await requireViewer();
  const webhooks = await getWebhooks();

  return (
    <section className="stack fade-up">
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
            <strong>{webhooks.items.filter((item) => item.status === "active").length}</strong>
          </div>
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
