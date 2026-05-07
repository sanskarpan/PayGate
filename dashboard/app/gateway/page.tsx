import { getActiveGatewayScenario, getGatewayScenarios, requireViewer } from "../../lib/api";
import { formatTime } from "../../lib/types";

function modeBadge(mode: string) {
  switch (mode) {
    case "success": return "badge-success";
    case "slow": return "badge-warning";
    case "flaky": return "badge-warning";
    case "timeout": return "badge-error";
    case "decline": return "badge-error";
    case "late_callback": return "badge-warning";
    default: return "badge-neutral";
  }
}

function modeDescription(mode: string, failureRate: number, delayMs: number) {
  switch (mode) {
    case "success": return "All authorizations succeed in ~50ms";
    case "slow": return `All authorizations succeed after ${delayMs}ms delay`;
    case "flaky": return `${Math.round(failureRate * 100)}% of authorizations randomly fail`;
    case "timeout": return "Authorizations never respond (30s timeout)";
    case "decline": return "All authorizations are declined";
    case "late_callback": return `Authorization succeeds after ${delayMs}ms (simulates late bank response)`;
    default: return mode;
  }
}

export default async function GatewaySimulatorPage() {
  await requireViewer();
  const [active, scenarios] = await Promise.all([
    getActiveGatewayScenario(),
    getGatewayScenarios(),
  ]);

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Gateway Simulator</div>
        <h1>Payment Gateway Control Panel</h1>
        <p className="lede">
          Configure the simulated payment gateway behavior for testing failure paths and edge cases.
        </p>
      </div>

      {active && (
        <div className="detail-card">
          <h2>Active Global Scenario</h2>
          <div style={{ marginTop: "1rem" }}>
            <div style={{ display: "flex", alignItems: "center", gap: "1rem" }}>
              <span className={modeBadge(active.mode)} style={{ fontSize: "1rem", padding: "0.4rem 1rem" }}>
                {active.mode}
              </span>
              <p className="lede" style={{ margin: 0 }}>
                {modeDescription(active.mode, active.failure_rate, active.delay_ms)}
              </p>
            </div>
          </div>
        </div>
      )}

      <div className="detail-card">
        <h2>Change Scenario</h2>
        <p className="muted">Use the API to configure the gateway simulator:</p>
        <div style={{ marginTop: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
          {[
            { mode: "success", label: "Happy Path", desc: "All payments succeed" },
            { mode: "slow", label: "Slow Gateway", desc: "3 second delay" },
            { mode: "flaky", label: "Flaky (30%)", desc: "30% random failures" },
            { mode: "decline", label: "All Declined", desc: "All payments fail" },
            { mode: "timeout", label: "Timeout", desc: "No response at all" },
          ].map(({ mode, label, desc }) => (
            <div key={mode} className="list-row" style={{ cursor: "default" }}>
              <div>
                <div className="row-title">{label}</div>
                <div className="row-meta">
                  <span className={modeBadge(mode)}>{mode}</span>
                  <span>{desc}</span>
                </div>
              </div>
              <code style={{ fontSize: "0.75rem", opacity: 0.7 }}>
                POST /v1/gateway/scenarios
              </code>
            </div>
          ))}
        </div>

        <div style={{ marginTop: "1.5rem" }}>
          <h3>API Example</h3>
          <pre style={{ background: "var(--surface)", padding: "1rem", borderRadius: "0.5rem", overflow: "auto", fontSize: "0.85rem" }}>
{`# Switch to flaky mode (30% failure rate)
curl -X POST http://localhost:8090/v1/gateway/scenarios \\
  -H 'Content-Type: application/json' \\
  -d '{"mode":"flaky","failure_rate":0.30,"delay_ms":0}'

# Switch back to success mode
curl -X POST http://localhost:8090/v1/gateway/scenarios \\
  -d '{"mode":"success"}'

# Check active scenario
curl http://localhost:8090/v1/gateway/scenarios/active`}
          </pre>
        </div>
      </div>

      <div className="list-card">
        <h2 style={{ padding: "1rem 1rem 0" }}>Scenario History</h2>
        {scenarios.items.length === 0 ? (
          <p className="muted" style={{ padding: "1rem" }}>No scenarios configured yet. Using default (success) mode.</p>
        ) : (
          scenarios.items.map((sc) => (
            <div className="list-row" key={sc.id}>
              <div>
                <div className="row-title">{sc.merchant_id || "Global"}</div>
                <div className="row-meta">
                  <span className={modeBadge(sc.mode)}>{sc.mode}</span>
                  {sc.mode === "flaky" && <span>{Math.round(sc.failure_rate * 100)}% failure</span>}
                  {(sc.mode === "slow" || sc.mode === "late_callback") && <span>{sc.delay_ms}ms delay</span>}
                  {sc.active && <span className="badge-success">active</span>}
                  <span>{formatTime(sc.created_at)}</span>
                </div>
              </div>
            </div>
          ))
        )}
      </div>
    </section>
  );
}
