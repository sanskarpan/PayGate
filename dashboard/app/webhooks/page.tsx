import Link from "next/link";

import { getWebhooks, requireViewer } from "../../lib/api";
import { formatTime } from "../../lib/types";

export default async function WebhooksPage() {
  const viewer = await requireViewer();
  const webhooks = await getWebhooks();

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Merchant Scope</div>
        <h1>Webhooks</h1>
        <p className="lede">
          {webhooks.count} subscription{webhooks.count !== 1 ? "s" : ""} configured for{" "}
          {viewer.merchant_id}.
        </p>
      </div>
      <div className="list-card">
        {webhooks.items.length === 0 ? (
          <p className="muted">No webhook subscriptions configured.</p>
        ) : (
          webhooks.items.map((wh) => (
            <Link className="list-row" href={`/webhooks/${wh.id}`} key={wh.id}>
              <div>
                <div className="row-title">{wh.url}</div>
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
