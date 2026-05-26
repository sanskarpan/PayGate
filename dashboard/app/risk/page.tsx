import { getRiskEvents, requireViewer } from "../../lib/api";
import RiskQueueManager from "../../components/risk-queue-manager";

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
      <RiskQueueManager initialItems={events.items} />
    </section>
  );
}
