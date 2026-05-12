import { redirect } from "next/navigation";

import { getApiBaseUrl, getAppBaseUrl, getViewerOptional } from "../lib/api";

export default async function LoginPage() {
  const viewer = await getViewerOptional();
  if (viewer) {
    redirect("/overview");
  }

  const action = `${getApiBaseUrl()}/v1/dashboard/login`;
  const redirectTo = `${getAppBaseUrl()}/overview`;

  return (
    <section className="auth-shell fade-up">
      <div className="auth-panel">
        <div className="eyebrow">Operator Access</div>
        <h1>Money movement, risk, and reliability in one control room.</h1>
        <p className="lede">
          Sign in with a merchant dashboard account to inspect orders, payment attempts,
          webhooks, settlements, disputes, payout activity, and operational health.
        </p>

        <div className="metric-strip">
          <div className="metric-chip">
            <span className="metric-chip-label">Surface</span>
            <strong>Payments</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Ops</span>
            <strong>Settlements</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Security</span>
            <strong>Keys + Team</strong>
          </div>
        </div>

        <form action={action} className="stack" method="POST" style={{ marginTop: "22px" }}>
          <input name="redirect_to" type="hidden" value={redirectTo} />
          <label>
            Merchant ID
            <input name="merchant_id" placeholder="merch_xxx" required type="text" />
          </label>
          <label>
            User Email
            <input name="email" placeholder="owner@example.com" required type="email" />
          </label>
          <label>
            Password
            <input name="password" placeholder="********" required type="password" />
          </label>
          <div className="hero-actions">
            <button className="primary-button" type="submit">
              Enter Control Room
            </button>
            <span className="micro-copy">
              Sessions are issued by the backend dashboard auth flow.
            </span>
          </div>
        </form>
      </div>

      <div className="auth-aside">
        <div className="spotlight-card">
          <div className="eyebrow">Operational Coverage</div>
          <h2>Purpose-built for payment operators</h2>
          <p className="lede">
            Review transaction timelines, investigate failures, manage webhook reliability,
            monitor reconciliation, and inspect disputes without leaving the dashboard.
          </p>
        </div>
        <div className="metric-card">
          <div className="eyebrow">Access Model</div>
          <div className="metric-value">Scoped</div>
          <p className="muted">
            Dashboard sessions respect merchant, role, and API-key policy boundaries.
          </p>
        </div>
        <div className="metric-card">
          <div className="eyebrow">Bootstrap Path</div>
          <p className="muted">
            Initial users can be created through the merchant bootstrap endpoint during setup,
            then managed from the team console.
          </p>
          <code>POST /v1/merchants/{"{merchant_id}"}/users/bootstrap</code>
        </div>
      </div>
    </section>
  );
}
