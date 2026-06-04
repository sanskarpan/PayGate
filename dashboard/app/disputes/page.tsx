import Link from "next/link";
import { getDisputes, requireViewer } from "../../lib/api";
import { formatMoney, formatTime } from "../../lib/types";
import { RouteIcon } from "../../components/system-icons";

function statusBadge(status: string) {
  switch (status) {
    case "won": return "badge-success";
    case "lost": return "badge-error";
    case "accepted": return "badge-warning";
    case "under_review": return "badge-warning";
    default: return "badge-neutral";
  }
}

function reasonLabel(reason: string) {
  const labels: Record<string, string> = {
    fraudulent: "Fraudulent",
    product_not_received: "Product Not Received",
    duplicate: "Duplicate Charge",
    product_unacceptable: "Product Unacceptable",
    credit_not_processed: "Credit Not Processed",
    general: "General",
  };
  return labels[reason] ?? reason;
}

export default async function DisputesPage() {
  const viewer = await requireViewer();
  const disputes = await getDisputes();

  const openCount = disputes.items.filter((d) => d.status === "open" || d.status === "under_review").length;
  const terminalCount = disputes.items.filter((d) => ["won", "lost", "accepted"].includes(d.status)).length;

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card has-glyph">
          <div className="page-glyph">
            <div className="page-glyph-badge">
              <RouteIcon name="disputes" size={40} />
            </div>
            <div className="page-glyph-label">
              <span>Case ledger</span>
              <strong>Dispute lane</strong>
            </div>
          </div>
          <div className="eyebrow">Chargeback Management</div>
          <h1>Disputes</h1>
          <p className="lede">
            {disputes.count} dispute{disputes.count !== 1 ? "s" : ""} for {viewer.merchant_id}.
            {openCount > 0 && (
              <span className="badge-error" style={{ marginLeft: "0.5rem" }}>
                {openCount} open
              </span>
            )}
          </p>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Case Pressure</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Open queue</span>
              <strong>{openCount}</strong>
              <p>Active disputes still requiring evidence, adjudication, or manual case progress.</p>
            </div>
            <div className="status-block">
              <span>Closed cases</span>
              <strong>{terminalCount}</strong>
              <p>Terminal disputes already settled into won, lost, or accepted outcomes.</p>
            </div>
            <div className="status-block">
              <span>Total inventory</span>
              <strong>{disputes.count}</strong>
              <p>All known disputes currently visible in the merchant operator boundary.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Total cases</span>
          <strong>{disputes.count}</strong>
          <span>All disputes currently tracked in the merchant scope.</span>
        </div>
        <div className="ops-band-item">
          <span>Open cases</span>
          <strong>{openCount}</strong>
          <span>Items still capable of affecting merchant loss posture.</span>
        </div>
        <div className="ops-band-item">
          <span>Terminal cases</span>
          <strong>{terminalCount}</strong>
          <span>Resolved or accepted cases no longer awaiting operator action.</span>
        </div>
        <div className="ops-band-item">
          <span>Merchant scope</span>
          <strong>{viewer.merchant_id.slice(-12)}</strong>
          <span>Current dispute queue tenant boundary.</span>
        </div>
      </div>

      <div className="list-card">
        <div className="section-head">
          <div>
            <h2>Case queue</h2>
            <div className="section-kicker">Chargeback state, reason, linked payment, and due time</div>
          </div>
        </div>
        {disputes.items.length === 0 ? (
          <div className="empty-state">
            <strong>No disputes recorded</strong>
            <span className="muted">Merchant chargebacks and retrieval flows will surface here when they occur.</span>
          </div>
        ) : (
          disputes.items.map((d) => (
            <Link className="list-row" href={`/disputes/${d.id}`} key={d.id}>
              <div>
                <div className="row-title">{d.id}</div>
                <div className="row-meta">
                  <span className={statusBadge(d.status)}>{d.status}</span>
                  <span>{reasonLabel(d.reason)}</span>
                  <span>Payment: {d.payment_id}</span>
                  {d.due_by && (
                    <span className="badge-warning">Due: {formatTime(d.due_by)}</span>
                  )}
                  <span>{formatTime(d.created_at)}</span>
                </div>
              </div>
              <div className="amount-pill">{formatMoney(d.amount, d.currency)}</div>
            </Link>
          ))
        )}
      </div>
    </section>
  );
}
