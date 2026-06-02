import Link from "next/link";

import { getUPIMandates, requireViewer } from "../../lib/api";
import { formatMoney, formatTime, truncateMiddle } from "../../lib/types";

export default async function MandatesPage() {
  await requireViewer();
  const mandates = await getUPIMandates();
  const active = mandates.items.filter((item) => item.status === "active").length;
  const pendingApproval = mandates.items.filter((item) => item.status === "pending_approval").length;
  const suspended = mandates.items.filter((item) => item.status === "paused" || item.status === "revoked" || item.status === "expired").length;

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card">
          <div className="eyebrow">UPI AutoPay</div>
          <h1>Mandates and recurring approvals.</h1>
          <p className="lede">Inspect mandate state, customer linkage, expiry windows, and recurring collection readiness.</p>
          <div className="metric-strip">
            <div className="metric-chip">
              <span className="metric-chip-label">Mandates</span>
              <strong>{mandates.count}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Active</span>
              <strong>{active}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Pending approval</span>
              <strong>{pendingApproval}</strong>
            </div>
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Mandate Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Active</span>
              <strong>{active}</strong>
              <p>Mandates currently allowed to drive recurring UPI collections.</p>
            </div>
            <div className="status-block">
              <span>Pending approval</span>
              <strong>{pendingApproval}</strong>
              <p>Customer approval flows that have not yet crossed into live recurring use.</p>
            </div>
            <div className="status-block">
              <span>Inactive pool</span>
              <strong>{suspended}</strong>
              <p>Paused, revoked, or expired mandate records still visible to operators.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="list-card">
        <div className="section-head">
          <div>
            <div className="eyebrow">Mandate Registry</div>
            <h2>Mandate registry</h2>
            <div className="section-kicker">Customer approvals, amount ceilings, and lifecycle state</div>
          </div>
        </div>
        {mandates.items.length === 0 ? (
          <div className="empty-state">
            <strong>No mandates created</strong>
            <span className="muted">Create a UPI AutoPay mandate to enable recurring collections for subscriptions.</span>
          </div>
        ) : (
          mandates.items.map((mandate) => (
            <article className="list-row" key={mandate.id}>
              <div className="entity-primary">
                <div className="row-title">
                  <Link href={`/mandates/${mandate.id}`}>{mandate.display_name}</Link>
                </div>
                <div className="row-meta">
                  <span className={mandate.status === "active" ? "badge-success" : mandate.status === "pending_approval" ? "badge-warning" : "badge-neutral"}>
                    {mandate.status}
                  </span>
                  <span>{truncateMiddle(mandate.reference, 12, 6)}</span>
                  <span>{mandate.vpa}</span>
                  <span>{formatMoney(mandate.amount_limit, mandate.currency)}</span>
                </div>
                <div className="muted">Created {formatTime(mandate.created_at)} · interval {mandate.interval_count} {mandate.interval_unit}</div>
              </div>
            </article>
          ))
        )}
      </div>
    </section>
  );
}
