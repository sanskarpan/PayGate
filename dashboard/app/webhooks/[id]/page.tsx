import { getWebhook, getWebhookDeliveries } from "../../../lib/api";
import { formatTime, truncateMiddle } from "../../../lib/types";

export default async function WebhookDetailPage({ params }: { params: { id: string } }) {
  const [wh, deliveries] = await Promise.all([
    getWebhook(params.id),
    getWebhookDeliveries(params.id),
  ]);

  return (
    <section className="stack fade-up">
      <div className="hero-card">
        <div className="eyebrow">Webhook Subscription</div>
        <h1>{truncateMiddle(wh.id, 14, 8)}</h1>
        <p className="mono" style={{ margin: "0 0 8px" }}>{wh.id}</p>
        <p className="lede">
          Delivering to <code>{wh.url}</code> · status: {wh.status}
        </p>
      </div>
      <div className="key-facts">
        <div className="key-fact">
          <span className="eyebrow">Status</span>
          <strong>{wh.status}</strong>
        </div>
        <div className="key-fact">
          <span className="eyebrow">Events</span>
          <strong>{wh.events.length}</strong>
        </div>
        <div className="key-fact">
          <span className="eyebrow">Deliveries</span>
          <strong>{deliveries.count}</strong>
        </div>
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
            <div className="empty-state">
              <strong>No delivery attempts</strong>
              <span className="muted">The endpoint exists, but no event delivery attempts have been recorded yet.</span>
            </div>
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
