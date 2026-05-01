import { getInvitations, requireViewer } from "../../lib/api";
import { formatTime } from "../../lib/types";

function statusBadge(status: string) {
  if (status === "accepted") return "badge-success";
  if (status === "pending") return "badge-warning";
  return "badge-error";
}

export default async function TeamPage() {
  const viewer = await requireViewer();
  const invitations = await getInvitations();

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Team Management</div>
        <h1>Team</h1>
        <p className="lede">
          {invitations.count} invitation{invitations.count !== 1 ? "s" : ""} for{" "}
          {viewer.merchant_id}.
        </p>
      </div>

      <div className="list-card">
        <div className="list-row">
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
        {invitations.items.length === 0 ? (
          <p className="muted">No invitations sent yet.</p>
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
