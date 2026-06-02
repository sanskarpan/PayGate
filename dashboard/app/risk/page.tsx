import { getRiskEvents, requireViewer } from "../../lib/api";
import RiskQueueManager from "../../components/risk-queue-manager";

export default async function RiskEventsPage() {
  const viewer = await requireViewer();
  const events = await getRiskEvents();
  const unresolved = events.items.filter((item) => !item.resolved).length;
  const inReview = events.items.filter((item) => item.review_status === "reviewed").length;
  const holdQueue = events.items.filter((item) => item.action === "hold" && !item.resolved).length;

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card">
          <div className="eyebrow">Risk Engine</div>
          <h1>Manual pressure queue.</h1>
          <p className="lede">
            {events.count} event{events.count !== 1 ? "s" : ""} recorded for {viewer.merchant_id}. Review holds, blocks,
            manual decisions, and operator resolution state from one work surface.
          </p>
          <div className="metric-strip">
            <div className="metric-chip">
              <span className="metric-chip-label">Open</span>
              <strong>{unresolved}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Reviewed</span>
              <strong>{inReview}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Hold queue</span>
              <strong>{holdQueue}</strong>
            </div>
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Queue Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Unresolved</span>
              <strong>{unresolved}</strong>
              <p>Events still requiring operator or policy resolution.</p>
            </div>
            <div className="status-block">
              <span>Hold decisions</span>
              <strong>{holdQueue}</strong>
              <p>Queued items that can affect capture timing or reserve posture.</p>
            </div>
            <div className="status-block">
              <span>Reviewed lane</span>
              <strong>{inReview}</strong>
              <p>Events that already passed through a manual review step.</p>
            </div>
          </div>
        </div>
      </div>
      <RiskQueueManager initialItems={events.items} />
    </section>
  );
}
