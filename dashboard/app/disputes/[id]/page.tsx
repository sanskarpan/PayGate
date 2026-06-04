import DisputeActionPanel from "../../../components/dispute-action-panel";
import { getApiBaseUrl, getDispute, requireViewer } from "../../../lib/api";
import { formatMoney, formatTime, truncateMiddle } from "../../../lib/types";

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
  const submitted = Boolean(dispute.evidence_submitted_at);
  const reviewStarted = dispute.status === "under_review" || isTerminal;
  const resolved = Boolean(dispute.resolved_at);
  const timeline = [
    { label: "Opened", at: dispute.created_at, active: true },
    { label: "Evidence submitted", at: dispute.evidence_submitted_at ?? 0, active: submitted },
    { label: "Under review", at: reviewStarted ? dispute.created_at : 0, active: reviewStarted },
    { label: "Resolved", at: dispute.resolved_at ?? 0, active: resolved },
  ];

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card">
          <div className="eyebrow">Dispute Detail</div>
          <h1>{truncateMiddle(dispute.id, 14, 8)}</h1>
          <p className="mono" style={{ margin: "0 0 8px" }}>{dispute.id}</p>
          <p className="lede">
            <span className={statusBadge(dispute.status)}>{dispute.status}</span>{" "}
            for {formatMoney(dispute.amount, dispute.currency)} tied to payment{" "}
            {truncateMiddle(dispute.payment_id)}.
          </p>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Case Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Status</span>
              <strong>{dispute.status}</strong>
              <p>Current dispute state across open, under-review, and terminal case outcomes.</p>
            </div>
            <div className="status-block">
              <span>Evidence posture</span>
              <strong>{submitted ? "Submitted" : "Pending"}</strong>
              <p>Whether the operator has provided a merchant-side evidence package.</p>
            </div>
            <div className="status-block">
              <span>Exposure</span>
              <strong>{formatMoney(dispute.amount, dispute.currency)}</strong>
              <p>Commercial amount currently at stake in the case workflow.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Status</span>
          <strong>{dispute.status}</strong>
          <span>Current case state.</span>
        </div>
        <div className="ops-band-item">
          <span>Reason</span>
          <strong>{dispute.reason}</strong>
          <span>Primary network or issuer rationale for the case.</span>
        </div>
        <div className="ops-band-item">
          <span>Evidence</span>
          <strong>{submitted ? "Submitted" : "Pending"}</strong>
          <span>Merchant response material submitted to the dispute lane.</span>
        </div>
        <div className="ops-band-item">
          <span>Resolution</span>
          <strong>{resolved ? formatTime(dispute.resolved_at || 0) : "Open"}</strong>
          <span>Timestamp of terminal resolution, if one exists.</span>
        </div>
      </div>

      <div className="detail-grid">
        <div className="detail-card">
          <h2>Case summary</h2>
          <dl className="detail-list">
            <div><dt>Payment ID</dt><dd>{truncateMiddle(dispute.payment_id)}</dd></div>
            <div><dt>Amount</dt><dd>{formatMoney(dispute.amount, dispute.currency)}</dd></div>
            <div><dt>Reason</dt><dd>{dispute.reason}</dd></div>
            <div><dt>Status</dt><dd><span className={statusBadge(dispute.status)}>{dispute.status}</span></dd></div>
            {dispute.due_by && (
              <div><dt>Evidence due by</dt><dd>{formatTime(dispute.due_by)}</dd></div>
            )}
            {dispute.evidence_submitted_at && (
              <div><dt>Evidence submitted</dt><dd>{formatTime(dispute.evidence_submitted_at)}</dd></div>
            )}
            {dispute.resolved_at && (
              <div><dt>Resolved at</dt><dd>{formatTime(dispute.resolved_at)}</dd></div>
            )}
            {dispute.settlement_id && (
              <div><dt>Settlement ID</dt><dd>{truncateMiddle(dispute.settlement_id)}</dd></div>
            )}
            {dispute.notes && (
              <div><dt>Notes</dt><dd>{dispute.notes}</dd></div>
            )}
            <div><dt>Created at</dt><dd>{formatTime(dispute.created_at)}</dd></div>
          </dl>
        </div>

        <div className="detail-card">
          <h2>Case progress</h2>
          <div className="timeline">
            {timeline.map((entry) => (
              <div className={`timeline-item${entry.active ? " active" : ""}`} key={entry.label}>
                <div className="timeline-dot" />
                <div>
                  <div className="row-title">{entry.label}</div>
                  <div className="muted">{entry.active ? formatTime(entry.at) : "Pending"}</div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      {dispute.evidence && Object.keys(dispute.evidence).length > 0 && (
        <div className="detail-card">
          <h2>Submitted evidence payload</h2>
          <pre>{JSON.stringify(dispute.evidence, null, 2)}</pre>
        </div>
      )}

      <DisputeActionPanel apiBaseUrl={getApiBaseUrl()} dispute={dispute} />
    </section>
  );
}
