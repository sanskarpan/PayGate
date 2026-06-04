import { getWebhook, getWebhookDeliveries } from "../../../lib/api";
import { formatTime, truncateMiddle } from "../../../lib/types";

export default async function WebhookDetailPage({ params }: { params: { id: string } }) {
  const [wh, deliveries] = await Promise.all([
    getWebhook(params.id),
    getWebhookDeliveries(params.id),
  ]);

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card">
          <div className="eyebrow">Webhook Subscription</div>
          <h1>{truncateMiddle(wh.id, 14, 8)}</h1>
          <p className="mono" style={{ margin: "0 0 8px" }}>{wh.id}</p>
          <p className="lede">
            Delivering to <code>{wh.url}</code> · status: {wh.status}
          </p>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Delivery Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Subscription state</span>
              <strong>{wh.status}</strong>
              <p>Current readiness state for outbound delivery and retry behavior.</p>
            </div>
            <div className="status-block">
              <span>Event fan-out</span>
              <strong>{wh.events.length}</strong>
              <p>Distinct event types allowed to flow through this subscription contract.</p>
            </div>
            <div className="status-block">
              <span>Delivery attempts</span>
              <strong>{deliveries.count}</strong>
              <p>Observed attempts across initial dispatch and retry sequences.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Status</span>
          <strong>{wh.status}</strong>
          <span>Current operational posture of the subscription.</span>
        </div>
        <div className="ops-band-item">
          <span>Events</span>
          <strong>{wh.events.length}</strong>
          <span>Events permitted by the merchant delivery contract.</span>
        </div>
        <div className="ops-band-item">
          <span>Deliveries</span>
          <strong>{deliveries.count}</strong>
          <span>Total attempts recorded on the subscription log.</span>
        </div>
        <div className="ops-band-item">
          <span>Endpoint</span>
          <strong>{truncateMiddle(wh.url, 16, 10)}</strong>
          <span>Destination currently receiving event payloads.</span>
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
