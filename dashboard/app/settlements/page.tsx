import Link from "next/link";

import { getSettlements, requireViewer } from "../../lib/api";
import { formatCompactNumber, formatMoney, formatTime, truncateMiddle } from "../../lib/types";

export default async function SettlementsPage() {
  const viewer = await requireViewer();
  const settlements = await getSettlements();

  return (
    <section className="stack fade-up">
      <div className="hero-card">
        <div className="eyebrow">Money Outbound</div>
        <h1>Settlement Reports</h1>
        <p className="lede">
          {settlements.count} settlement batch{settlements.count !== 1 ? "es" : ""} for{" "}
          {viewer.merchant_id}.
        </p>
        <div className="metric-strip">
          <div className="metric-chip">
            <span className="metric-chip-label">Batches</span>
            <strong>{formatCompactNumber(settlements.count)}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Latest</span>
            <strong>{settlements.items[0]?.status ?? "Not run"}</strong>
          </div>
        </div>
      </div>
      <div className="list-card">
        <div className="section-head">
          <div>
            <h2>Settlement batches</h2>
            <div className="section-kicker">Net payout, batch state, payment count, and run time</div>
          </div>
        </div>
        {settlements.items.length === 0 ? (
          <div className="empty-state">
            <strong>No settlement batches yet</strong>
            <span className="muted">Once settlements are run, payout-ready batches will appear here.</span>
          </div>
        ) : (
          settlements.items.map((s) => (
            <Link className="list-row" href={`/settlements/${s.id}`} key={s.id}>
              <div className="entity-primary">
                <div className="row-title">{truncateMiddle(s.id)}</div>
                <div className="mono">{s.id}</div>
                <div className="row-meta">
                  <span className={s.status === "processed" ? "badge-success" : "badge-warning"}>
                    {s.status}
                  </span>
                  <span>{s.payment_count} payment{s.payment_count !== 1 ? "s" : ""}</span>
                  <span>{formatTime(s.created_at)}</span>
                </div>
              </div>
              <div className="amount-pill">{formatMoney(s.net_amount, s.currency)}</div>
            </Link>
          ))
        )}
      </div>
    </section>
  );
}
