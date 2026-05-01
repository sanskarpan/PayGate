import { getRiskEvents, requireViewer } from "../../lib/api";
import { formatTime } from "../../lib/types";

function actionBadge(action: string) {
  if (action === "block") return "badge-error";
  if (action === "hold") return "badge-warning";
  return "badge-success";
}

export default async function RiskEventsPage() {
  const viewer = await requireViewer();
  const events = await getRiskEvents();

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Risk Engine</div>
        <h1>Risk Events</h1>
        <p className="lede">
          {events.count} event{events.count !== 1 ? "s" : ""} recorded for {viewer.merchant_id}.
        </p>
      </div>
      <div className="list-card">
        {events.items.length === 0 ? (
          <p className="muted">No risk events recorded yet.</p>
        ) : (
          events.items.map((ev) => (
            <div className="list-row" key={ev.id}>
              <div>
                <div className="row-title">{ev.payment_id}</div>
                <div className="row-meta">
                  <span className={actionBadge(ev.action)}>{ev.action}</span>
                  <span>score: {ev.score}</span>
                  {ev.triggered_rules.length > 0 && (
                    <span>{ev.triggered_rules.join(", ")}</span>
                  )}
                  {ev.resolved && (
                    <span className="badge-success">resolved</span>
                  )}
                  <span>{formatTime(ev.created_at)}</span>
                </div>
              </div>
              <div className="amount-pill">{ev.id}</div>
            </div>
          ))
        )}
      </div>
    </section>
  );
}
