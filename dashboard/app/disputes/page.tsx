import Link from "next/link";
import { getDisputes, requireViewer } from "../../lib/api";
import { formatMoney, formatTime } from "../../lib/types";

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

  return (
    <section className="stack">
      <div className="hero-card">
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
      <div className="list-card">
        {disputes.items.length === 0 ? (
          <p className="muted">No disputes recorded. Great news!</p>
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
