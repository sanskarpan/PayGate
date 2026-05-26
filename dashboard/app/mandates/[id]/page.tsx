import { getUPIMandate, getUPIMandateEvents } from "../../../lib/api";
import { formatMoney, formatTime, truncateMiddle } from "../../../lib/types";

export default async function MandateDetailPage({ params }: { params: { id: string } }) {
  const [mandate, events] = await Promise.all([getUPIMandate(params.id), getUPIMandateEvents(params.id)]);

  return (
    <section className="stack fade-up">
      <div className="hero-card">
        <div className="eyebrow">Mandate Detail</div>
        <h1>{mandate.display_name}</h1>
        <p className="mono" style={{ margin: "0 0 8px" }}>{mandate.id}</p>
        <p className="lede">
          {mandate.status} · {formatMoney(mandate.amount_limit, mandate.currency)} ceiling · {mandate.vpa}
        </p>
      </div>

      <div className="detail-grid">
        <div className="detail-card">
          <h2>Attributes</h2>
          <dl className="detail-list">
            <div><dt>Reference</dt><dd>{truncateMiddle(mandate.reference)}</dd></div>
            <div><dt>Customer ID</dt><dd>{truncateMiddle(mandate.customer_id)}</dd></div>
            <div><dt>Interval</dt><dd>{mandate.interval_count} {mandate.interval_unit}</dd></div>
            <div><dt>Retry Window</dt><dd>{mandate.retry_window_hours}h</dd></div>
            <div><dt>Approved At</dt><dd>{mandate.approved_at ? formatTime(mandate.approved_at) : "Not approved"}</dd></div>
            <div><dt>Expires At</dt><dd>{mandate.expires_at ? formatTime(mandate.expires_at) : "No hard expiry"}</dd></div>
          </dl>
        </div>

        <div className="detail-card">
          <h2>History</h2>
          <div className="timeline">
            {events.items.map((event) => (
              <div className="timeline-item active" key={event.id}>
                <div className="timeline-dot" />
                <div>
                  <div className="row-title">{event.event_type}</div>
                  <div className="muted">{formatTime(event.created_at)} · {event.actor_type} · {event.actor_id || "system"}</div>
                  {event.reason ? <div className="muted">{event.reason}</div> : null}
                  {event.payment_id ? <div className="muted">payment {truncateMiddle(event.payment_id)}</div> : null}
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
