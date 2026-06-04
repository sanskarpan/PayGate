import Image from "next/image";
import { redirect } from "next/navigation";

import { SignalStamp } from "../components/system-icons";
import { getApiBaseUrl, getAppBaseUrl, getViewerOptional } from "../lib/api";

export default async function LoginPage() {
  let viewer = null;
  try {
    viewer = await getViewerOptional();
  } catch {
    viewer = null;
  }
  if (viewer) {
    redirect("/overview");
  }

  const action = `${getApiBaseUrl()}/v1/dashboard/login`;
  const redirectTo = `${getAppBaseUrl()}/overview`;

  return (
    <section className="landing-shell fade-up">
      <div className="landing-main">
        <div className="landing-copy">
          <div className="eyebrow">Merchant Control Surface</div>
          <div className="landing-headline">
            <span className="headline-slash" />
            <h1>
              Money
              <br />
              movement
              <br />
              in authored
              <br />
              command.
            </h1>
          </div>
          <p className="lede">
            A payment operations surface for orders, captures, webhooks, treasury, risk,
            disputes, and settlement posture without falling back to generic internal-tool UI.
          </p>
          <div className="landing-cta-row">
            <button className="primary-button" form="dashboard-login-form" type="submit">
              Enter Control Room
            </button>
            <span className="micro-copy">
              Merchant-scoped dashboard sessions issued by the backend auth flow.
            </span>
          </div>
          <div className="ticker-cloud">
            {[
              "Payments",
              "Webhooks",
              "Treasury",
              "Risk",
              "Reconciliation",
              "Disputes",
              "UPI",
              "Ledger",
            ].map((item) => (
              <span className="ticker-pill" key={item}>
                {item}
              </span>
            ))}
          </div>
        </div>

        <div className="landing-side">
          <div className="mesh-panel">
            <div className="mesh-badge">
              <SignalStamp size={78} />
            </div>
            <Image
              alt=""
              className="mesh-illustration"
              fill
              priority
              src="/hero-tunnel.svg"
            />
          </div>
        </div>
      </div>

      <div className="landing-foot">
        <div className="landing-foot-cell">
          <span className="status-label">Surface</span>
          <strong>Orders, payouts, webhooks, disputes</strong>
        </div>
        <div className="landing-foot-cell">
          <span className="status-label">Scope</span>
          <strong>Operator-focused merchant infrastructure</strong>
        </div>
        <div className="landing-foot-cell">
          <span className="status-label">Access</span>
          <strong>Scoped sessions, roles, and admin boundaries</strong>
        </div>
      </div>

      <div className="landing-auth-grid">
        <div className="auth-panel">
          <div className="section-head">
            <div>
              <div className="eyebrow">Operator Sign-In</div>
              <h2>Access merchant command lanes</h2>
            </div>
          </div>
          <form action={action} className="stack" id="dashboard-login-form" method="POST" style={{ marginTop: "22px" }}>
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
                Log In
              </button>
              <span className="micro-copy">Use an existing merchant dashboard account.</span>
            </div>
          </form>
        </div>

        <div className="auth-aside">
          <div className="spotlight-card">
            <div className="eyebrow">Coverage</div>
            <h2>Built for money infrastructure operators</h2>
            <p className="lede">
              Review transaction timelines, inspect payout approvals, drive gateway scenarios,
              monitor reconciliation drift, and track webhook delivery without switching surfaces.
            </p>
          </div>
          <div className="metric-card">
            <div className="eyebrow">Bootstrap</div>
            <p className="muted">
              Initial users can be created through the merchant bootstrap endpoint during setup,
              then managed from the team console.
            </p>
            <code>POST /v1/merchants/{"{merchant_id}"}/users/bootstrap</code>
          </div>
        </div>
      </div>
    </section>
  );
}
