import { getEventSchemas, getLedgerHolds, getSagas, requireViewer } from "../../lib/api";
import { formatMoney, formatTime, truncateMiddle } from "../../lib/types";

function tone(status: string) {
  switch (status) {
    case "completed":
    case "active":
    case "acked":
      return "badge-success";
    case "failed":
    case "dead_lettered":
    case "nacked":
    case "expired":
      return "badge-error";
    case "running":
    case "pending":
    case "leased":
      return "badge-warning";
    default:
      return "badge-neutral";
  }
}

export default async function ControlPlanePage() {
  const viewer = await requireViewer();
  const [sagas, schemas, holds] = await Promise.all([getSagas(), getEventSchemas(), getLedgerHolds()]);
  const openHolds = holds.items.filter((item) => item.status === "active").length;
  const failedSagas = sagas.items.filter((item) => item.status === "failed" || item.status === "dead_lettered").length;
  const activeSchemas = schemas.items.length;

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card">
          <div className="eyebrow">Runtime Control Plane</div>
          <h1>Async, schemas, and held funds.</h1>
          <p className="lede">Inspect orchestration, event-schema governance, and ledger controls for {viewer.merchant_id}.</p>
          <div className="metric-strip">
            <div className="metric-chip">
              <span className="metric-chip-label">Tracked sagas</span>
              <strong>{sagas.count}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Schema subjects</span>
              <strong>{schemas.count}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Active holds</span>
              <strong>{openHolds}</strong>
            </div>
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Runtime Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Failed workflows</span>
              <strong>{failedSagas}</strong>
              <p>Sagas currently failed or dead-lettered and likely needing operator intervention.</p>
            </div>
            <div className="status-block">
              <span>Governed subjects</span>
              <strong>{activeSchemas}</strong>
              <p>Event schema subjects under active compatibility and rollout control.</p>
            </div>
            <div className="status-block">
              <span>Held funds</span>
              <strong>{openHolds}</strong>
              <p>Reserved balances currently blocking or shaping downstream treasury movement.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Saga inventory</span>
          <strong>{sagas.count}</strong>
          <span>Total orchestrated long-running workflows visible to operators.</span>
        </div>
        <div className="ops-band-item">
          <span>Schema surface</span>
          <strong>{schemas.count}</strong>
          <span>Subjects with explicit topic and ownership control.</span>
        </div>
        <div className="ops-band-item">
          <span>Active holds</span>
          <strong>{openHolds}</strong>
          <span>Reserve and ledger holds currently in force.</span>
        </div>
        <div className="ops-band-item">
          <span>Failure pressure</span>
          <strong>{failedSagas}</strong>
          <span>Workflows already in failure or dead-letter lanes.</span>
        </div>
      </div>

      <div className="detail-grid">
        <div className="list-card">
          <div className="section-head">
            <div>
              <h2>Sagas</h2>
              <div className="section-kicker">Replay and override asynchronous business workflows</div>
            </div>
          </div>
          {sagas.items.length === 0 ? (
            <div className="empty-state">
              <strong>No saga instances</strong>
              <span className="muted">Long-running workflows will surface here as the control plane operates.</span>
            </div>
          ) : (
            sagas.items.map((item) => (
              <article className="list-row" key={item.id}>
                <div className="entity-primary">
                  <div className="row-title">{item.saga_type}</div>
                  <div className="mono">{truncateMiddle(item.id)}</div>
                  <div className="row-meta">
                    <span className={tone(item.status)}>{item.status}</span>
                    <span>step {item.current_step_index}</span>
                    <span>replays {item.replay_count}</span>
                    <span>{formatTime(item.created_at)}</span>
                  </div>
                  {item.failure_reason ? <div className="muted">{item.failure_reason}</div> : null}
                </div>
                <div className="row-actions">
                  <form action={`/api/proxy/v1/sagas/${item.id}/replay`} method="POST">
                    <input type="hidden" name="_redirect" value="/control-plane" />
                    <button className="ghost-button" type="submit">Replay</button>
                  </form>
                  <form action={`/api/proxy/v1/sagas/${item.id}/override`} method="POST">
                    <input type="hidden" name="_redirect" value="/control-plane" />
                    <input type="hidden" name="action" value="abort" />
                    <button className="ghost-button" type="submit">Abort</button>
                  </form>
                </div>
              </article>
            ))
          )}
        </div>

        <div className="list-card">
          <div className="section-head">
            <div>
              <h2>Event schemas</h2>
              <div className="section-kicker">Current schema subjects and topic ownership</div>
            </div>
          </div>
          {schemas.items.map((item) => (
            <div className="list-row" key={item.id}>
              <div>
                <div className="row-title">{item.subject}</div>
                <div className="row-meta">
                  <span className="badge-info">{item.event_type}</span>
                  <span>{item.topic_name}</span>
                  <span>{item.owner}</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="detail-card">
        <div className="section-head">
          <div>
            <h2>Operator runbook</h2>
            <div className="section-kicker">Common interventions across orchestration and ledger surfaces</div>
          </div>
        </div>
        <div className="command-deck" style={{ marginTop: "16px" }}>
          <div className="command-link">
            <strong>Replay failed saga</strong>
            <span>Use replay when a transient dependency or downstream service recovered after an earlier failure.</span>
          </div>
          <div className="command-link">
            <strong>Abort dangerous workflow</strong>
            <span>Use abort when a still-running saga should not continue after state divergence or policy intervention.</span>
          </div>
          <div className="command-link">
            <strong>Commit or release holds</strong>
            <span>Use ledger holds to explicitly shape reserve outcomes rather than letting latent states drift unnoticed.</span>
          </div>
        </div>
      </div>

      <div className="list-card">
        <div className="section-head">
          <div>
            <h2>Ledger holds</h2>
            <div className="section-kicker">Reserved balances and explicit commit or release actions</div>
          </div>
        </div>
        {holds.items.length === 0 ? (
          <div className="empty-state">
            <strong>No ledger holds</strong>
            <span className="muted">Reserve and treasury controls will appear here once created.</span>
          </div>
        ) : (
          holds.items.map((item) => (
            <article className="list-row" key={item.id}>
              <div className="entity-primary">
                <div className="row-title">{item.reason}</div>
                <div className="row-meta">
                  <span className={tone(item.status)}>{item.status}</span>
                  <span>{item.account_code}</span>
                  <span>{item.source_type}:{item.source_id}</span>
                  <span>{formatTime(item.created_at)}</span>
                </div>
              </div>
              <div className="row-actions">
                <div className="amount-pill">{formatMoney(item.amount, item.currency)}</div>
                {item.status === "active" ? (
                  <>
                    <form action={`/api/proxy/v1/ledger/holds/${item.id}/release`} method="POST">
                      <input type="hidden" name="_redirect" value="/control-plane" />
                      <button className="ghost-button" type="submit">Release</button>
                    </form>
                    <form action={`/api/proxy/v1/ledger/holds/${item.id}/commit`} method="POST">
                      <input type="hidden" name="_redirect" value="/control-plane" />
                      <input type="hidden" name="target_account_code" value="MERCHANT_PAYABLE" />
                      <input type="hidden" name="description" value="dashboard commit" />
                      <button className="ghost-button" type="submit">Commit</button>
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
