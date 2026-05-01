import Link from "next/link";

import { getSettlements, requireViewer } from "../../lib/api";
import { formatMoney, formatTime } from "../../lib/types";

export default async function SettlementsPage() {
  const viewer = await requireViewer();
  const settlements = await getSettlements();

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Merchant Scope</div>
        <h1>Settlement Reports</h1>
        <p className="lede">
          {settlements.count} settlement batch{settlements.count !== 1 ? "es" : ""} for{" "}
          {viewer.merchant_id}.
        </p>
      </div>
      <div className="list-card">
        {settlements.items.length === 0 ? (
          <p className="muted">No settlement batches have been run yet.</p>
        ) : (
          settlements.items.map((s) => (
            <Link className="list-row" href={`/settlements/${s.id}`} key={s.id}>
              <div>
                <div className="row-title">{s.id}</div>
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
