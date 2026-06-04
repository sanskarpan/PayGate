import { getReconMismatches, requireViewer } from "../../lib/api";
import { formatCompactNumber, formatTime, truncateMiddle } from "../../lib/types";

export default async function ReconPage() {
  const viewer = await requireViewer();
  const mismatches = await getReconMismatches();

  const open = mismatches.items.filter((m) => !m.resolved);
  const resolved = mismatches.items.filter((m) => m.resolved);

  const mismatchTypeLabel: Record<string, string> = {
    ledger_imbalance: "Ledger Imbalance",
    payment_missing_ledger: "Missing Ledger Entry",
    payment_amount_mismatch: "Amount Mismatch",
    settlement_payment_mismatch: "Settlement Mismatch",
    payment_settled_not_in_batch: "Settled Without Batch",
  };

  const groupedOpen = open.reduce<Record<string, number>>((acc, item) => {
    acc[item.mismatch_type] = (acc[item.mismatch_type] ?? 0) + 1;
    return acc;
  }, {});

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card">
          <div className="eyebrow">Reconciliation</div>
          <h1>Reconciliation Control</h1>
          <p className="lede">
            {open.length} open mismatch{open.length !== 1 ? "es" : ""} · {resolved.length} resolved
            for {viewer.merchant_id}.
          </p>
          <div className="metric-strip">
            <div className="metric-chip">
              <span className="metric-chip-label">Open</span>
              <strong>{formatCompactNumber(open.length)}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Resolved</span>
              <strong>{formatCompactNumber(resolved.length)}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Distinct mismatch types</span>
              <strong>{Object.keys(groupedOpen).length}</strong>
            </div>
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Control Assessment</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Open anomaly set</span>
              <strong>{open.length}</strong>
              <p>Current mismatches that can block finance confidence or merchant closeout.</p>
            </div>
            <div className="status-block">
              <span>Resolved history</span>
              <strong>{resolved.length}</strong>
              <p>Historical anomalies already acknowledged, corrected, or operationally accepted.</p>
            </div>
            <div className="status-block">
              <span>Type breadth</span>
              <strong>{Object.keys(groupedOpen).length}</strong>
              <p>Distinct mismatch categories currently represented in the open queue.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Open mismatches</span>
          <strong>{open.length}</strong>
          <span>Items still requiring operator acknowledgement or remediation.</span>
        </div>
        <div className="ops-band-item">
          <span>Resolved mismatches</span>
          <strong>{resolved.length}</strong>
          <span>Historical items closed out by manual or systemic correction.</span>
        </div>
        <div className="ops-band-item">
          <span>Mismatch classes</span>
          <strong>{Object.keys(groupedOpen).length}</strong>
          <span>Number of distinct mismatch patterns present in the current queue.</span>
        </div>
        <div className="ops-band-item">
          <span>Merchant scope</span>
          <strong>{truncateMiddle(viewer.merchant_id)}</strong>
          <span>Current reconciliation surface is scoped to this operator tenant.</span>
        </div>
      </div>

      {open.length > 0 ? (
        <div className="intel-grid">
          {Object.entries(groupedOpen).map(([type, count]) => (
            <div className="intel-card" key={type}>
              <span>Mismatch type</span>
              <strong>{count}</strong>
              <p>{mismatchTypeLabel[type] ?? type}</p>
            </div>
          ))}
        </div>
      ) : null}

      {open.length > 0 && (
        <div className="list-card">
          <div className="section-head">
            <div>
              <h2>Open mismatches</h2>
              <div className="section-kicker">Expected value, actual value, entity scope, and detection time</div>
            </div>
          </div>
          {open.map((mm) => (
            <div className="list-row list-row-error" key={mm.id}>
              <div className="entity-primary">
                <div className="row-title">
                  {mismatchTypeLabel[mm.mismatch_type] ?? mm.mismatch_type}
                </div>
                <div className="mono">{truncateMiddle(mm.id)}</div>
                <div className="row-meta">
                  <span>{mm.entity_type}: {mm.entity_id}</span>
                  <span>Expected: {mm.expected_value}</span>
                  <span>Actual: {mm.actual_value}</span>
                  <span>{formatTime(mm.created_at)}</span>
                </div>
                {mm.description && <div className="muted">{mm.description}</div>}
              </div>
            </div>
          ))}
        </div>
      )}

      {open.length === 0 && (
        <div className="success-panel">
          <div className="eyebrow">Healthy State</div>
          <p className="lede">All reconciliation checks passed. No open mismatches.</p>
        </div>
      )}

      {resolved.length > 0 && (
        <div className="list-card">
          <div className="section-head">
            <div>
              <h2>Resolved mismatches</h2>
              <div className="section-kicker">Historical anomalies already acknowledged or corrected</div>
            </div>
          </div>
          {resolved.map((mm) => (
            <div className="list-row" key={mm.id}>
              <div className="entity-primary">
                <div className="row-title">
                  {mismatchTypeLabel[mm.mismatch_type] ?? mm.mismatch_type}
                </div>
                <div className="mono">{truncateMiddle(mm.id)}</div>
                <div className="row-meta">
                  <span>{mm.entity_type}: {mm.entity_id}</span>
                  <span>{formatTime(mm.created_at)}</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
