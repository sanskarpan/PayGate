import {
  getCapabilities,
  getOnboarding,
  getOnboardingDocuments,
  getOnboardingParties,
  getReserveEscalations,
  getReservePolicy,
  getScreeningCases,
  requireViewer,
} from "../../lib/api";
import { formatMoney, formatTime } from "../../lib/types";
import ComplianceOpsConsole from "../../components/compliance-ops-console";
import { RouteIcon } from "../../components/system-icons";

function stateTone(state: string) {
  switch (state) {
    case "approved":
    case "passed":
    case "enabled":
    case "verified":
      return "badge-success";
    case "rejected":
    case "failed":
    case "disabled":
      return "badge-error";
    case "submitted":
    case "in_review":
    case "review":
    case "restricted":
    case "pending":
      return "badge-warning";
    default:
      return "badge-neutral";
  }
}

export default async function CompliancePage() {
  const viewer = await requireViewer();
  const [onboarding, parties, documents, screenings, capabilities, reservePolicy, escalations] =
    await Promise.all([
      getOnboarding(),
      getOnboardingParties(),
      getOnboardingDocuments(),
      getScreeningCases(),
      getCapabilities(),
      getReservePolicy(),
      getReserveEscalations(),
    ]);

  const verifiedOwners = parties.items.filter((item) => item.verification_status === "verified").length;
  const approvedDocs = documents.items.filter((item) => item.status === "approved").length;
  const pendingEscalations = escalations.items.filter((item) => item.status === "pending").length;
  const enabledCapabilities = capabilities.items.filter((item) => item.status === "enabled").length;

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card has-glyph">
          <div className="page-glyph">
            <div className="page-glyph-badge">
              <RouteIcon name="compliance" size={40} />
            </div>
            <div className="page-glyph-label">
              <span>KYB signal</span>
              <strong>Control lane</strong>
            </div>
          </div>
          <div className="eyebrow">Merchant Compliance</div>
          <h1>Onboarding and controls.</h1>
          <p className="lede">
            Review onboarding completeness, capability posture, and reserve escalations for {viewer.merchant_id}.
          </p>
          <div className="metric-strip">
            <div className="metric-chip">
              <span className="metric-chip-label">Application state</span>
              <strong>{onboarding.state}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Verified parties</span>
              <strong>{verifiedOwners}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Approved documents</span>
              <strong>{approvedDocs}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Pending escalations</span>
              <strong>{pendingEscalations}</strong>
            </div>
          </div>
          <div className="hero-actions">
            <form action="/api/proxy/v1/merchants/me/onboarding/submit" method="POST">
              <input type="hidden" name="_redirect" value="/compliance" />
              <button className="primary-button" type="submit" disabled={onboarding.state === "approved"}>
                Submit onboarding
              </button>
            </form>
            <form action="/api/proxy/v1/merchants/me/onboarding/screenings/run" method="POST">
              <input type="hidden" name="_redirect" value="/compliance" />
              <input type="hidden" name="screening_type" value="kyb" />
              <input type="hidden" name="force_result" value="passed" />
              <button className="ghost-button" type="submit">Run screening</button>
            </form>
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Control Assessment</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Application lane</span>
              <strong>{onboarding.state}</strong>
              <p>{onboarding.submitted_at ? `Submitted ${formatTime(onboarding.submitted_at)}.` : "Still in draft and not yet in formal review."}</p>
            </div>
            <div className="status-block">
              <span>Reserve posture</span>
              <strong>{reservePolicy.policy_type}</strong>
              <p>{reservePolicy.percentage_bps / 100}% reserve held for {reservePolicy.hold_days} days at current policy.</p>
            </div>
            <div className="status-block">
              <span>Capability exposure</span>
              <strong>{enabledCapabilities}</strong>
              <p>Payment and treasury capabilities currently enabled for live merchant behavior.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Application</span>
          <strong>{onboarding.state}</strong>
          <span>{onboarding.submitted_at ? `Submitted ${formatTime(onboarding.submitted_at)}` : "Draft application"}</span>
        </div>
        <div className="ops-band-item">
          <span>Reserve policy</span>
          <strong>{reservePolicy.policy_type}</strong>
          <span>{reservePolicy.percentage_bps / 100}% · {reservePolicy.hold_days} day hold</span>
        </div>
        <div className="ops-band-item">
          <span>Capabilities</span>
          <strong>{enabledCapabilities}</strong>
          <span>Enabled payment and treasury capabilities.</span>
        </div>
        <div className="ops-band-item">
          <span>Threshold</span>
          <strong>{formatMoney(reservePolicy.threshold_amount, "INR")}</strong>
          <span>Current reserve escalation trigger.</span>
        </div>
      </div>

      <ComplianceOpsConsole merchantID={viewer.merchant_id} onboarding={onboarding} capabilities={capabilities.items} />

      <div className="detail-grid">
        <div className="list-card">
          <div className="section-head">
            <div>
              <h2>Ownership and control parties</h2>
              <div className="section-kicker">Beneficial owners and controllers captured for KYB review</div>
            </div>
          </div>
          {parties.items.map((party) => (
            <div className="list-row" key={party.id}>
              <div>
                <div className="row-title">{party.full_name}</div>
                <div className="row-meta">
                  <span className="badge-neutral">{party.party_type}</span>
                  <span className={stateTone(party.verification_status)}>{party.verification_status}</span>
                  {party.ownership_bps ? <span>{party.ownership_bps / 100}% ownership</span> : null}
                  {party.email ? <span>{party.email}</span> : null}
                </div>
              </div>
              <div className="amount-pill">rev {party.revision}</div>
            </div>
          ))}
        </div>

        <div className="list-card">
          <div className="section-head">
            <div>
              <h2>Document queue</h2>
              <div className="section-kicker">Requested, uploaded, and approved onboarding evidence</div>
            </div>
          </div>
          {documents.items.map((doc) => (
            <div className="list-row" key={doc.id}>
              <div>
                <div className="row-title">{doc.document_type}</div>
                <div className="row-meta">
                  <span className={stateTone(doc.status)}>{doc.status}</span>
                  {doc.file_name ? <span>{doc.file_name}</span> : <span>Awaiting upload</span>}
                  {doc.expires_at ? <span>expires {doc.expires_at}</span> : null}
                </div>
                {doc.request_reason ? <div className="muted">{doc.request_reason}</div> : null}
                {doc.review_notes ? <div className="muted">{doc.review_notes}</div> : null}
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="detail-grid">
        <div className="list-card">
          <div className="section-head">
            <div>
              <h2>Screening cases</h2>
              <div className="section-kicker">Sanctions, KYB, and policy checks</div>
            </div>
          </div>
          {screenings.items.length === 0 ? (
            <div className="empty-state">
              <strong>No screening cases</strong>
              <span className="muted">Run screening to create the first compliance review case.</span>
            </div>
          ) : (
            screenings.items.map((item) => (
              <div className="list-row" key={item.id}>
                <div>
                  <div className="row-title">{item.screening_type}</div>
                  <div className="row-meta">
                    <span className={stateTone(item.status)}>{item.status}</span>
                    <span>{item.provider}</span>
                    <span>{item.subject_name}</span>
                    <span>{formatTime(item.screened_at)}</span>
                  </div>
                </div>
              </div>
            ))
          )}
        </div>

        <div className="list-card">
          <div className="section-head">
            <div>
              <h2>Capability posture</h2>
              <div className="section-kicker">What the merchant can currently do in live flows</div>
            </div>
          </div>
          {capabilities.items.map((item) => (
            <div className="list-row" key={item.id}>
              <div>
                <div className="row-title">{item.capability_code}</div>
                <div className="row-meta">
                  <span className={stateTone(item.status)}>{item.status}</span>
                  {item.reason ? <span>{item.reason}</span> : null}
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="list-card">
        <div className="section-head">
          <div>
            <h2>Reserve escalations</h2>
            <div className="section-kicker">Risk-triggered reserve recommendations awaiting decision</div>
          </div>
        </div>
        {escalations.items.length === 0 ? (
          <div className="empty-state">
            <strong>No reserve escalations</strong>
            <span className="muted">Risk events have not suggested additional reserve action yet.</span>
          </div>
        ) : (
          escalations.items.map((item) => (
            <article className="list-row" key={item.id}>
              <div className="entity-primary">
                <div className="row-title">Risk event {item.risk_event_id}</div>
                <div className="row-meta">
                  <span className={stateTone(item.status)}>{item.status}</span>
                  <span>score {item.trigger_score}</span>
                  <span>{item.suggested_policy_type}</span>
                  <span>{item.suggested_percentage_bps / 100}%</span>
                  <span>{item.suggested_hold_days} day hold</span>
                </div>
                <div className="muted">{item.rationale}</div>
              </div>
              {item.status === "pending" ? (
                <div className="row-actions">
                  <form action={`/api/proxy/v1/merchants/me/reserve-escalations/${item.id}/review`} method="POST">
                    <input type="hidden" name="_redirect" value="/compliance" />
                    <input type="hidden" name="decision" value="approved" />
                    <button className="ghost-button" type="submit">Approve</button>
                  </form>
                  <form action={`/api/proxy/v1/merchants/me/reserve-escalations/${item.id}/review`} method="POST">
                    <input type="hidden" name="_redirect" value="/compliance" />
                    <input type="hidden" name="decision" value="rejected" />
                    <button className="ghost-button" type="submit">Reject</button>
                  </form>
                </div>
              ) : null}
            </article>
          ))
        )}
      </div>
    </section>
  );
}
