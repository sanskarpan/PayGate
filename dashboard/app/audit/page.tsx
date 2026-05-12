import { getAuditLogs, requireViewer } from "../../lib/api";
import { formatTime } from "../../lib/types";

export default async function AuditLogPage() {
  const viewer = await requireViewer();
  const logs = await getAuditLogs();

  return (
    <section className="stack fade-up">
      <div className="hero-card">
        <div className="eyebrow">Compliance</div>
        <h1>Audit Log</h1>
        <p className="lede">
          {logs.count} event{logs.count !== 1 ? "s" : ""} recorded for {viewer.merchant_id}.
        </p>
      </div>
      <div className="list-card">
        <div className="section-head">
          <div>
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
