import { getWebhook, getWebhookDeliveries } from "../../../lib/api";
import { formatTime } from "../../../lib/types";

export default async function WebhookDetailPage({ params }: { params: { id: string } }) {
  const [wh, deliveries] = await Promise.all([
    getWebhook(params.id),
    getWebhookDeliveries(params.id),
  ]);

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Webhook Subscription</div>
        <h1>{wh.id}</h1>
        <p className="lede">
          Delivering to <code>{wh.url}</code> · status: {wh.status}
        </p>
      </div>
      <div className="detail-grid">
        <div className="detail-card">
          <h2>Subscription Details</h2>
          <dl className="detail-list">
            <div>
              <dt>Events</dt>
              <dd>{wh.events.join(", ")}</dd>
            </div>
            <div>
              <dt>Status</dt>
              <dd>{wh.status}</dd>
            </div>
            <div>
              <dt>Created</dt>
              <dd>{formatTime(wh.created_at)}</dd>
            </div>
            <div>
              <dt>Last Updated</dt>
              <dd>{formatTime(wh.updated_at)}</dd>
            </div>
          </dl>
        </div>
        <div className="detail-card">
          <h2>Delivery Log</h2>
          {deliveries.items.length === 0 ? (
            <p className="muted">No delivery attempts recorded yet.</p>
          ) : (
            <div className="timeline">
              {deliveries.items.map((attempt) => (
                <div
                  key={attempt.id}
                  className={`timeline-item${attempt.status === "succeeded" ? " active" : ""}`}
                >
                  <div className="timeline-dot" />
                  <div>
                    <div className="row-title">
                      Attempt #{attempt.attempt_number} ·{" "}
                      <span
                        className={
                          attempt.status === "succeeded" ? "badge-success" : "badge-warning"
                        }
                      >
                        {attempt.status}
                      </span>
                    </div>
                    <div className="row-meta">
                      {attempt.response_code ? (
                        <span>HTTP {attempt.response_code}</span>
                      ) : null}
                      {attempt.error ? <span className="error">{attempt.error}</span> : null}
                      <span>{formatTime(attempt.created_at)}</span>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
