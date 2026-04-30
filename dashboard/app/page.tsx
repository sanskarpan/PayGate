import { redirect } from "next/navigation";

import { getApiBaseUrl, getAppBaseUrl, getViewerOptional } from "../lib/api";

export default async function LoginPage() {
  const viewer = await getViewerOptional();
  if (viewer) {
    redirect("/orders");
  }

  const action = `${getApiBaseUrl()}/v1/dashboard/login`;
  const redirectTo = `${getAppBaseUrl()}/orders`;
  return (
    <section className="auth-shell">
      <div className="auth-panel">
        <div className="eyebrow">Phase 1 Dashboard</div>
        <h1>Merchant Control Room</h1>
        <p className="lede">
          Authenticate with a bootstrapped merchant user. Use the merchant ID, user email, and password
          created through the backend bootstrap endpoint.
        </p>
        <form action={action} className="stack" method="POST">
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
          <button className="primary-button" type="submit">
            Sign In
          </button>
        </form>
        <p className="muted">
          Bootstrap a dashboard user with <code>POST /v1/merchants/{"{merchant_id}"}/users/bootstrap</code>.
        </p>
      </div>
      <div className="auth-aside">
        <div className="metric-card">
          <div className="eyebrow">Live Views</div>
          <div className="metric-value">Orders</div>
          <p>Server-rendered list, detail, and linked payment traces.</p>
        </div>
        <div className="metric-card">
          <div className="eyebrow">Operator Workflow</div>
          <div className="metric-value">API Keys</div>
          <p>Create and revoke credentials using the same backend policies as public APIs.</p>
        </div>
      </div>
    </section>
  );
}
