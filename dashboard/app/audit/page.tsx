import { getAuditLogs, requireViewer } from "../../lib/api";
import { RouteIcon } from "../../components/system-icons";
import { formatTime } from "../../lib/types";

export default async function AuditLogPage() {
  const viewer = await requireViewer();
  const logs = await getAuditLogs();
  const actors = new Set(logs.items.map((log) => log.actor_email || log.actor_id).filter(Boolean)).size;

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card has-glyph">
          <div className="page-glyph">
            <div className="page-glyph-badge">
              <RouteIcon name="audit" size={40} />
            </div>
            <div className="page-glyph-label">
              <span>Evidence log</span>
              <strong>Trace rail</strong>
            </div>
          </div>
          <div className="eyebrow">Trace Ledger</div>
          <h1>Audit event trail.</h1>
          <p className="lede">
            {logs.count} event{logs.count !== 1 ? "s" : ""} recorded for {viewer.merchant_id}. Review operator actions,
            resource changes, actor identity, and source IP from one evidentiary lane.
          </p>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Review Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Total events</span>
              <strong>{logs.count}</strong>
              <p>Protected operations and state-changing actions currently retained.</p>
            </div>
            <div className="status-block">
              <span>Actors seen</span>
              <strong>{actors}</strong>
              <p>Distinct identities present in the visible audit window.</p>
            </div>
            <div className="status-block">
              <span>Merchant scope</span>
              <strong>{viewer.merchant_id}</strong>
              <p>Tenant boundary currently loaded into this evidentiary surface.</p>
            </div>
          </div>
        </div>
      </div>
      <div className="ops-band">
        <div className="ops-band-item">
          <span>Total traces</span>
          <strong>{logs.count}</strong>
          <span>Retained audit records available in the current evidence window.</span>
        </div>
        <div className="ops-band-item">
          <span>Actors seen</span>
          <strong>{actors}</strong>
          <span>Distinct identities present in the current audit slice.</span>
        </div>
        <div className="ops-band-item">
          <span>Surface scope</span>
          <strong>{viewer.merchant_id.slice(-12)}</strong>
          <span>Tenant evidence boundary loaded into this review session.</span>
        </div>
        <div className="ops-band-item">
          <span>Signal type</span>
          <strong>Mutations</strong>
          <span>Protected actions, actor identity, IP source, and timing records.</span>
        </div>
      </div>
      <div className="list-card">
        <div className="section-head">
          <div>
            <div className="eyebrow">Recorded Actions</div>
            <h2>Recorded actions</h2>
            <div className="section-kicker">Actor, resource, source IP, and event timestamp</div>
          </div>
        </div>
        {logs.items.length === 0 ? (
          <div className="empty-state">
            <strong>No audit events recorded</strong>
            <span className="muted">Protected operations will start appearing here once they occur.</span>
          </div>
        ) : (
          logs.items.map((log) => (
            <div className="list-row" key={log.id}>
              <div>
                <div className="row-title">
                  {log.action} — {log.resource_type}
                  {log.resource_id ? ` / ${log.resource_id}` : ""}
                </div>
                <div className="row-meta">
                  <span>{log.actor_email || log.actor_id}</span>
                  <span className="badge-info">{log.actor_type}</span>
                  {log.ip_address && <span>{log.ip_address}</span>}
                  <span>{formatTime(log.created_at)}</span>
                </div>
              </div>
              <div style={{ fontSize: "0.78rem", color: "var(--muted)", fontFamily: "monospace" }}>{log.id.slice(0, 8)}</div>
            </div>
          ))
        )}
      </div>
    </section>
  );
}
