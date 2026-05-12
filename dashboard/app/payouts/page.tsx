import { getPayouts, requireViewer } from "../../lib/api";
import { formatMoney, formatTime } from "../../lib/types";

function statusBadge(status: string) {
  switch (status) {
    case "completed":
      return "badge-success";
    case "failed":
      return "badge-error";
    case "processing":
      return "badge-warning";
    default:
      return "badge-neutral";
  }
}

export default async function PayoutsPage() {
  const viewer = await requireViewer();
  const payouts = await getPayouts();
  const completed = payouts.items.filter((item) => item.status === "completed").length;

  return (
    <section className="stack fade-up">
      <div className="hero-card">
        <div className="eyebrow">Bank Transfers</div>
        <h1>Payouts</h1>
        <p className="lede">
          {payouts.count} payout{payouts.count !== 1 ? "s" : ""} for {viewer.merchant_id}.
        </p>
        <div className="metric-strip">
          <div className="metric-chip">
            <span className="metric-chip-label">Completed</span>
            <strong>{completed}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Pending / failed</span>
            <strong>{payouts.count - completed}</strong>
          </div>
        </div>
      </div>
      <div className="list-card">
        {payouts.items.length === 0 ? (
          <div className="empty-state">
            <strong>No payouts initiated</strong>
            <span className="muted">
              Run a settlement batch and then initiate a payout through the settlement payout endpoint.
            </span>
            <code>POST /v1/settlements/{"{id}"}/payout</code>
          </div>
        ) : (
          payouts.items.map((p) => (
            <div className="list-row" key={p.id}>
              <div>
                <div className="row-title">{p.id}</div>
                <div className="row-meta">
                  <span className={statusBadge(p.status)}>{p.status}</span>
                  <span>Settlement: {p.settlement_id}</span>
                  {p.bank_reference && <span>Ref: {p.bank_reference}</span>}
                  {p.failure_reason && (
                    <span className="badge-error">{p.failure_reason}</span>
                  )}
                  <span>{formatTime(p.created_at)}</span>
                </div>
              </div>
              <div className="amount-pill">{formatMoney(p.amount, p.currency)}</div>
            </div>
          ))
        )}
      </div>
    </section>
  );
}
