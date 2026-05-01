import { getReconMismatches, requireViewer } from "../../lib/api";
import { formatTime } from "../../lib/types";

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

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Reconciliation</div>
        <h1>Recon Dashboard</h1>
        <p className="lede">
          {open.length} open mismatch{open.length !== 1 ? "es" : ""} · {resolved.length} resolved
          for {viewer.merchant_id}.
        </p>
      </div>

      {open.length > 0 && (
        <div className="list-card">
          <h2 style={{ padding: "16px 20px", margin: 0, borderBottom: "1px solid var(--border)" }}>
            Open Mismatches ({open.length})
          </h2>
          {open.map((mm) => (
            <div className="list-row" key={mm.id} style={{ borderLeft: "3px solid var(--error)" }}>
              <div>
                <div className="row-title">
                  {mismatchTypeLabel[mm.mismatch_type] ?? mm.mismatch_type}
                </div>
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
        <div className="hero-card" style={{ background: "var(--success-bg)" }}>
          <p className="lede">All reconciliation checks passed. No open mismatches.</p>
        </div>
      )}

      {resolved.length > 0 && (
        <div className="list-card">
          <h2 style={{ padding: "16px 20px", margin: 0, borderBottom: "1px solid var(--border)" }}>
            Resolved ({resolved.length})
          </h2>
          {resolved.map((mm) => (
            <div className="list-row" key={mm.id}>
              <div>
                <div className="row-title">
                  {mismatchTypeLabel[mm.mismatch_type] ?? mm.mismatch_type}
                </div>
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
