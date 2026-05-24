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
  const unresolved = events.items.filter((item) => !item.resolved).length;
  const inReview = events.items.filter((item) => item.review_status === "reviewed").length;

  return (
    <section className="stack fade-up">
      <div className="hero-card">
        <div className="eyebrow">Risk Engine</div>
        <h1>Risk Events</h1>
        <p className="lede">
          {events.count} event{events.count !== 1 ? "s" : ""} recorded for {viewer.merchant_id}.
        </p>
        <div className="metric-strip">
          <div className="metric-chip">
            <span className="metric-chip-label">Unresolved</span>
            <strong>{unresolved}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Resolved</span>
            <strong>{events.count - unresolved}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Reviewed</span>
            <strong>{inReview}</strong>
          </div>
        </div>
      </div>
      <div className="list-card">
        <div className="section-head">
          <div>
            <h2>Event queue</h2>
            <div className="section-kicker">Action, score, triggered rules, and resolution state</div>
          </div>
        </div>
        {events.items.length === 0 ? (
          <div className="empty-state">
            <strong>No risk events recorded</strong>
            <span className="muted">The fraud and policy engine has not raised any merchant events yet.</span>
          </div>
        ) : (
          events.items.map((ev) => (
            <article className="list-row" key={ev.id}>
              <div>
                <div className="row-title">{ev.payment_id}</div>
                <div className="row-meta">
                  <span className={actionBadge(ev.action)}>{ev.action}</span>
                  <span>score: {ev.score}</span>
                  {ev.review_status ? <span>{ev.review_status}</span> : null}
                  {ev.assigned_to ? <span>assigned to {ev.assigned_to}</span> : null}
                  {ev.manual_decision ? <span>decision: {ev.manual_decision}</span> : null}
                  {ev.triggered_rules?.length > 0 && (
                    <span>{ev.triggered_rules.join(", ")}</span>
                  )}
                  {ev.resolved && (
                    <span className="badge-success">resolved</span>
                  )}
                  <span>{formatTime(ev.created_at)}</span>
                </div>
                {ev.review_notes ? <div className="muted">{ev.review_notes}</div> : null}
              </div>
              <div className="row-actions">
                <div className="amount-pill">Score: {ev.score}</div>
                {!ev.assigned_to ? (
                  <form action={`/api/proxy/v1/risk/events/${ev.id}/assign`} method="POST">
                    <input type="hidden" name="_redirect" value="/risk" />
                    <button className="ghost-button" type="submit">Assign</button>
                  </form>
                ) : null}
                {!ev.resolved ? (
                  <>
                    <form action={`/api/proxy/v1/risk/events/${ev.id}/review`} method="POST">
                      <input type="hidden" name="_redirect" value="/risk" />
                      <input type="hidden" name="decision" value="approve" />
                      <button className="ghost-button" type="submit">Approve</button>
                    </form>
                    <form action={`/api/proxy/v1/risk/events/${ev.id}/review`} method="POST">
                      <input type="hidden" name="_redirect" value="/risk" />
                      <input type="hidden" name="decision" value="block" />
                      <button className="ghost-button" type="submit">Block</button>
                    </form>
                    <form action={`/api/proxy/v1/risk/events/${ev.id}/resolve`} method="POST">
                      <input type="hidden" name="_redirect" value="/risk" />
                      <button className="ghost-button" type="submit">Resolve</button>
                    </form>
                  </>
                ) : null}
              </div>
            </article>
          ))
        )}
      </div>
    </section>
  );
}
