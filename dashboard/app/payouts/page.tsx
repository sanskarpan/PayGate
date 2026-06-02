import { getBeneficiaries, getPayouts, requireViewer } from "../../lib/api";
import { RouteIcon } from "../../components/system-icons";
import { formatMoney, formatTime } from "../../lib/types";
import PayoutApprovalManager from "../../components/payout-approval-manager";

function statusBadge(status: string) {
  switch (status) {
    case "completed":
      return "badge-success";
    case "failed":
      return "badge-error";
    case "processing":
      return "badge-warning";
    default:
      return "badge-neutral";
  }
}

export default async function PayoutsPage() {
  const viewer = await requireViewer();
  const [payouts, beneficiaries] = await Promise.all([getPayouts(), getBeneficiaries()]);
  const completed = payouts.items.filter((item) => item.status === "completed").length;
  const pendingApprovals = payouts.items.filter((item) => item.approval_status === "pending").length;
  const verifiedBeneficiaries = beneficiaries.items.filter((item) => item.status === "approved").length;

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card has-glyph">
          <div className="page-glyph">
            <div className="page-glyph-badge">
              <RouteIcon name="payouts" size={40} />
            </div>
            <div className="page-glyph-label">
              <span>Treasury graph</span>
              <strong>Outbound rail</strong>
            </div>
          </div>
          <div className="eyebrow">Treasury Rail</div>
          <h1>Bank payout operations.</h1>
          <p className="lede">
            {payouts.count} payout{payouts.count !== 1 ? "s" : ""} for {viewer.merchant_id}. Manage destination approval, treasury
            readiness, and transfer queue decisions from one surface.
          </p>
          <div className="metric-strip">
            <div className="metric-chip">
              <span className="metric-chip-label">Completed</span>
              <strong>{completed}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Approval queue</span>
              <strong>{pendingApprovals}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Approved beneficiaries</span>
              <strong>{verifiedBeneficiaries}</strong>
            </div>
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Treasury Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Payout queue</span>
              <strong>{payouts.count - completed}</strong>
              <p>Transfers still pending approval, processing, or recovery.</p>
            </div>
            <div className="status-block">
              <span>Destination trust</span>
              <strong>{verifiedBeneficiaries}</strong>
              <p>Beneficiaries already approved for live treasury routing.</p>
            </div>
            <div className="status-block">
              <span>Merchant scope</span>
              <strong>{viewer.merchant_id}</strong>
              <p>Current merchant treasury perimeter loaded in this operator session.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Total transfers</span>
          <strong>{payouts.count}</strong>
          <span>Visible payout attempts within the current treasury perimeter.</span>
        </div>
        <div className="ops-band-item">
          <span>Approvals pending</span>
          <strong>{pendingApprovals}</strong>
          <span>Transfers still waiting for a manual treasury decision.</span>
        </div>
        <div className="ops-band-item">
          <span>Approved destinations</span>
          <strong>{verifiedBeneficiaries}</strong>
          <span>Beneficiaries trusted for live settlement disbursement.</span>
        </div>
        <div className="ops-band-item">
          <span>Merchant scope</span>
          <strong>{viewer.merchant_id.slice(-12)}</strong>
          <span>Current treasury tenant boundary loaded into the session.</span>
        </div>
      </div>

      <div className="detail-grid">
        <div className="glass-card">
          <div className="section-head">
            <div>
              <div className="eyebrow">Destination Setup</div>
              <h2>Create beneficiary</h2>
              <div className="section-kicker">Treasury destination for settlement payouts</div>
            </div>
          </div>
          <form action="/api/proxy/v1/beneficiaries" className="stack" method="POST" style={{ marginTop: "16px" }}>
            <input type="hidden" name="_redirect" value="/payouts" />
            <div className="inline-form">
              <label>
                Destination
                <select name="destination_type" defaultValue="bank_account">
                  <option value="bank_account">bank_account</option>
                  <option value="vpa">vpa</option>
                </select>
              </label>
              <label>
                Holder name
                <input name="account_holder_name" type="text" />
              </label>
            </div>
            <div className="inline-form">
              <label>
                Bank account
                <input name="bank_account_number" type="text" />
              </label>
              <label>
                IFSC
                <input name="bank_ifsc" type="text" />
              </label>
              <label>
                VPA
                <input name="vpa" placeholder="merchant@upi" type="text" />
              </label>
            </div>
            <button className="primary-button" type="submit">Create beneficiary</button>
          </form>
        </div>

        <div className="list-card">
          <div className="section-head">
            <div>
              <div className="eyebrow">Destination Inventory</div>
              <h2>Beneficiaries</h2>
              <div className="section-kicker">Verification and approval status for payout destinations</div>
            </div>
          </div>
          {beneficiaries.items.length === 0 ? (
            <div className="empty-state">
              <strong>No beneficiaries configured</strong>
              <span className="muted">Create and verify a beneficiary before running treasury payouts.</span>
            </div>
          ) : (
            beneficiaries.items.map((item) => (
              <article className="list-row" key={item.id}>
                <div className="entity-primary">
                  <div className="row-title">{item.account_holder_name}</div>
                  <div className="row-meta">
                    <span className={statusBadge(item.status)}>{item.status}</span>
                    <span>{item.destination_type}</span>
                    {item.bank_account_last4 ? <span>•••• {item.bank_account_last4}</span> : null}
                    {item.vpa ? <span>{item.vpa}</span> : null}
                  </div>
                </div>
                <div className="row-actions">
                  <form action={`/api/proxy/v1/beneficiaries/${item.id}/verify`} method="POST">
                    <input type="hidden" name="_redirect" value="/payouts" />
                    <button className="ghost-button" type="submit">Verify</button>
                  </form>
                  <form action={`/api/proxy/v1/beneficiaries/${item.id}/approve`} method="POST">
                    <input type="hidden" name="_redirect" value="/payouts" />
                    <button className="ghost-button" type="submit">Approve</button>
                  </form>
                </div>
              </article>
            ))
          )}
        </div>
      </div>
      <PayoutApprovalManager items={payouts.items} />
    </section>
  );
}
