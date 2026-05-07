import { getDispute, requireViewer } from "../../../lib/api";
import { formatMoney, formatTime } from "../../../lib/types";

function statusBadge(status: string) {
  switch (status) {
    case "won": return "badge-success";
    case "lost": return "badge-error";
    case "accepted": return "badge-warning";
    case "under_review": return "badge-warning";
    default: return "badge-neutral";
  }
}

export default async function DisputeDetailPage({ params }: { params: { id: string } }) {
  await requireViewer();
  const dispute = await getDispute(params.id);

  const isTerminal = ["won", "lost", "accepted"].includes(dispute.status);

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Dispute Detail</div>
        <h1>{dispute.id}</h1>
        <p className="lede">
          <span className={statusBadge(dispute.status)}>{dispute.status}</span>
        </p>
      </div>

      <div className="detail-card">
        <dl className="detail-grid">
          <dt>Payment ID</dt>
          <dd>{dispute.payment_id}</dd>
          <dt>Amount</dt>
          <dd>{formatMoney(dispute.amount, dispute.currency)}</dd>
          <dt>Reason</dt>
          <dd>{dispute.reason}</dd>
          <dt>Status</dt>
          <dd><span className={statusBadge(dispute.status)}>{dispute.status}</span></dd>
          {dispute.due_by && (
            <>
              <dt>Evidence Due By</dt>
              <dd>{formatTime(dispute.due_by)}</dd>
            </>
          )}
          {dispute.evidence_submitted_at && (
            <>
              <dt>Evidence Submitted</dt>
              <dd>{formatTime(dispute.evidence_submitted_at)}</dd>
            </>
          )}
          {dispute.resolved_at && (
            <>
              <dt>Resolved At</dt>
              <dd>{formatTime(dispute.resolved_at)}</dd>
            </>
          )}
          {dispute.settlement_id && (
            <>
              <dt>Settlement ID</dt>
              <dd>{dispute.settlement_id}</dd>
            </>
          )}
          {dispute.notes && (
            <>
              <dt>Notes</dt>
              <dd>{dispute.notes}</dd>
            </>
          )}
          <dt>Created At</dt>
          <dd>{formatTime(dispute.created_at)}</dd>
        </dl>

        {dispute.evidence && Object.keys(dispute.evidence).length > 0 && (
          <div style={{ marginTop: "1.5rem" }}>
            <h3>Submitted Evidence</h3>
            <pre style={{ background: "var(--surface)", padding: "1rem", borderRadius: "0.5rem", overflow: "auto", fontSize: "0.85rem" }}>
              {JSON.stringify(dispute.evidence, null, 2)}
            </pre>
          </div>
        )}

        {!isTerminal && (
          <div style={{ marginTop: "1.5rem" }}>
            <p className="muted">
              Use the API to submit evidence, start review, or resolve this dispute.
            </p>
            <div className="row-meta" style={{ marginTop: "0.5rem" }}>
              {dispute.status === "open" && (
                <>
                  <code>POST /v1/disputes/{dispute.id}/submit-evidence</code>
                  <code>POST /v1/disputes/{dispute.id}/review</code>
                  <code>POST /v1/disputes/{dispute.id}/accept</code>
                </>
              )}
              {dispute.status === "under_review" && (
                <code>POST /v1/disputes/{dispute.id}/resolve {`{"outcome":"won"}`}</code>
              )}
            </div>
          </div>
        )}
      </div>
    </section>
  );
}
