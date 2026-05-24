import { getBeneficiaries, getPayouts, requireViewer } from "../../lib/api";
import { formatMoney, formatTime } from "../../lib/types";

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

  return (
    <section className="stack fade-up">
      <div className="hero-card">
        <div className="eyebrow">Bank Transfers</div>
        <h1>Payouts</h1>
        <p className="lede">
          {payouts.count} payout{payouts.count !== 1 ? "s" : ""} for {viewer.merchant_id}.
        </p>
        <div className="metric-strip">
          <div className="metric-chip">
            <span className="metric-chip-label">Completed</span>
            <strong>{completed}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Pending / failed</span>
            <strong>{payouts.count - completed}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Approval queue</span>
            <strong>{pendingApprovals}</strong>
          </div>
        </div>
      </div>
      <div className="detail-grid">
        <div className="glass-card">
          <div className="section-head">
            <div>
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
      <div className="list-card">
        {payouts.items.length === 0 ? (
          <div className="empty-state">
            <strong>No payouts initiated</strong>
            <span className="muted">
              Run a settlement batch and then initiate a payout through the settlement payout endpoint.
            </span>
            <code>POST /v1/settlements/{"{id}"}/payout</code>
          </div>
        ) : (
          payouts.items.map((p) => (
            <article className="list-row" key={p.id}>
              <div>
                <div className="row-title">{p.id}</div>
                <div className="row-meta">
                  <span className={statusBadge(p.status)}>{p.status}</span>
                  {p.approval_status ? <span>{p.approval_status}</span> : null}
                  <span>Settlement: {p.settlement_id}</span>
                  {p.beneficiary_id ? <span>Beneficiary: {p.beneficiary_id}</span> : null}
                  {p.bank_reference && <span>Ref: {p.bank_reference}</span>}
                  {p.failure_reason && (
                    <span className="badge-error">{p.failure_reason}</span>
                  )}
                  <span>{formatTime(p.created_at)}</span>
                </div>
              </div>
              <div className="row-actions">
                <div className="amount-pill">{formatMoney(p.amount, p.currency)}</div>
                {p.approval_status === "pending" ? (
                  <>
                    <form action={`/api/proxy/v1/payouts/${p.id}/approve`} method="POST">
                      <input type="hidden" name="_redirect" value="/payouts" />
                      <button className="ghost-button" type="submit">Approve</button>
                    </form>
                    <form action={`/api/proxy/v1/payouts/${p.id}/reject`} method="POST">
                      <input type="hidden" name="_redirect" value="/payouts" />
                      <button className="ghost-button" type="submit">Reject</button>
                    </form>
                  </>
                ) : null}
              </div>
            </article>
          ))
        )}
      </div>
    </section>
  );
}
