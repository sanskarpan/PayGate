import Link from "next/link";

import { getSettlements, requireViewer } from "../../lib/api";
import { formatCompactNumber, formatMoney, formatTime, truncateMiddle } from "../../lib/types";

export default async function SettlementsPage() {
  const viewer = await requireViewer();
  const settlements = await getSettlements();
  const processedCount = settlements.items.filter((item) => item.status === "processed").length;
  const latestNet = settlements.items[0]?.net_amount ?? 0;

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
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

        <div className="detail-card">
          <div className="eyebrow">Treasury Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Processed batches</span>
              <strong>{processedCount}</strong>
              <p>Batches already finalized and moved through downstream treasury handling.</p>
            </div>
            <div className="status-block">
              <span>Latest net</span>
              <strong>{settlements.items[0] ? formatMoney(latestNet, settlements.items[0].currency) : "Not run"}</strong>
              <p>Most recent merchant-facing net payout outcome across visible settlement history.</p>
            </div>
            <div className="status-block">
              <span>Batch inventory</span>
              <strong>{settlements.count}</strong>
              <p>Known settlement batches in the current commercial scope.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Batches</span>
          <strong>{settlements.count}</strong>
          <span>Total settlement records visible in this merchant scope.</span>
        </div>
        <div className="ops-band-item">
          <span>Processed</span>
          <strong>{processedCount}</strong>
          <span>Settlement batches already moved through the processed state.</span>
        </div>
        <div className="ops-band-item">
          <span>Latest status</span>
          <strong>{settlements.items[0]?.status ?? "Not run"}</strong>
          <span>Current posture of the most recently generated batch.</span>
        </div>
        <div className="ops-band-item">
          <span>Tenant</span>
          <strong>{truncateMiddle(viewer.merchant_id)}</strong>
          <span>Merchant boundary for the settlement feed.</span>
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
