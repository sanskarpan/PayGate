import { getAuditLogs, requireViewer } from "../../lib/api";
import { formatTime } from "../../lib/types";

export default async function AuditLogPage() {
  const viewer = await requireViewer();
  const logs = await getAuditLogs();

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Compliance</div>
        <h1>Audit Log</h1>
        <p className="lede">
          {logs.count} event{logs.count !== 1 ? "s" : ""} recorded for {viewer.merchant_id}.
        </p>
      </div>
      <div className="list-card">
        {logs.items.length === 0 ? (
          <p className="muted">No audit events recorded yet.</p>
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
              <div className="amount-pill">{log.id}</div>
            </div>
          ))
        )}
      </div>
    </section>
  );
}
