import { getInvitations, requireViewer } from "../../lib/api";
import { RouteIcon } from "../../components/system-icons";
import { formatTime } from "../../lib/types";

function statusBadge(status: string) {
  if (status === "accepted") return "badge-success";
  if (status === "pending") return "badge-warning";
  return "badge-error";
}

export default async function TeamPage() {
  const viewer = await requireViewer();
  const invitations = await getInvitations();
  const pending = invitations.items.filter((item) => item.status === "pending").length;

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card has-glyph">
          <div className="page-glyph">
            <div className="page-glyph-badge">
              <RouteIcon name="team" size={40} />
            </div>
            <div className="page-glyph-label">
              <span>Identity mesh</span>
              <strong>Access rail</strong>
            </div>
          </div>
          <div className="eyebrow">Operator Identity</div>
          <h1>Team access fabric.</h1>
          <p className="lede">
            {invitations.count} invitation{invitations.count !== 1 ? "s" : ""} for {viewer.merchant_id}. Invite operations,
            developer, and read-only users without leaving the merchant command surface.
          </p>
          <div className="metric-strip">
            <div className="metric-chip">
              <span className="metric-chip-label">Pending invites</span>
              <strong>{pending}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Accepted</span>
              <strong>{invitations.items.filter((item) => item.status === "accepted").length}</strong>
            </div>
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Access Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Merchant scope</span>
              <strong>{viewer.merchant_id}</strong>
              <p>Current tenant boundary for invitations and role assignment.</p>
            </div>
            <div className="status-block">
              <span>Pending</span>
              <strong>{pending}</strong>
              <p>Invitations still waiting for recipient action or revocation.</p>
            </div>
            <div className="status-block">
              <span>Accepted</span>
              <strong>{invitations.items.filter((item) => item.status === "accepted").length}</strong>
              <p>Operator identities already admitted into this merchant surface.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Total invites</span>
          <strong>{invitations.count}</strong>
          <span>Recorded operator invitations across the current merchant tenant.</span>
        </div>
        <div className="ops-band-item">
          <span>Pending handoffs</span>
          <strong>{pending}</strong>
          <span>Invites still awaiting acceptance or revocation.</span>
        </div>
        <div className="ops-band-item">
          <span>Accepted</span>
          <strong>{invitations.items.filter((item) => item.status === "accepted").length}</strong>
          <span>Operator identities already admitted into this surface.</span>
        </div>
        <div className="ops-band-item">
          <span>Tenant scope</span>
          <strong>{viewer.merchant_id.slice(-12)}</strong>
          <span>Current identity boundary loaded into the dashboard session.</span>
        </div>
      </div>

      <div className="glass-card">
        <div className="section-head">
          <div>
            <div className="eyebrow">Invite Operator</div>
            <h2>Send team invitation</h2>
          </div>
        </div>
        <div className="list-row" style={{ marginTop: "16px" }}>
          <form action="/api/invite" method="POST" className="inline-form">
            <input
              type="email"
              name="email"
              placeholder="colleague@example.com"
              required
              className="input"
            />
            <select name="role" className="input">
              <option value="developer">Developer</option>
              <option value="readonly">Read-only</option>
              <option value="ops">Operations</option>
            </select>
            <button type="submit" className="action-button">
              Send Invite
            </button>
          </form>
        </div>
      </div>

      <div className="list-card">
        <div className="section-head">
          <div>
            <div className="eyebrow">Invitation Inventory</div>
            <h2>Issued invites</h2>
          </div>
        </div>
        {invitations.items.length === 0 ? (
          <div className="empty-state">
            <strong>No invitations sent</strong>
            <span className="muted">Invite developers, ops staff, or read-only reviewers here.</span>
          </div>
        ) : (
          invitations.items.map((inv) => (
            <div className="list-row" key={inv.id}>
              <div>
                <div className="row-title">{inv.email}</div>
                <div className="row-meta">
                  <span className={statusBadge(inv.status)}>{inv.status}</span>
                  <span>{inv.role}</span>
                  <span>invited by {inv.invited_by}</span>
                  <span>expires {formatTime(inv.expires_at)}</span>
                </div>
              </div>
              {inv.status === "pending" && (
                <form action={`/api/revoke-invite/${inv.id}`} method="POST">
                  <button type="submit" className="ghost-button">
                    Revoke
                  </button>
                </form>
              )}
            </div>
          ))
        )}
      </div>
    </section>
  );
}
