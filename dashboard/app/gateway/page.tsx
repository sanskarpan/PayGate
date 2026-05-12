import GatewayScenarioControl from "../../components/gateway-scenario-control";
import { getActiveGatewayScenario, getApiBaseUrl, getGatewayScenarios, requireViewer } from "../../lib/api";
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
    <section className="stack fade-up">
      <div className="hero-card">
        <div className="eyebrow">Gateway Simulator</div>
        <h1>Payment Gateway Control Panel</h1>
        <p className="lede">
          Configure simulated gateway behavior for failure-path testing, latency drills, and
          callback timing edge cases.
        </p>
        <div className="metric-strip">
          <div className="metric-chip">
            <span className="metric-chip-label">Active mode</span>
            <strong>{active?.mode ?? "success"}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Saved scenarios</span>
            <strong>{scenarios.count}</strong>
          </div>
        </div>
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

      <GatewayScenarioControl
        apiBaseUrl={getApiBaseUrl()}
        initialActive={active}
        initialItems={scenarios.items}
      />

      <div className="detail-card">
        <h2>API Example</h2>
        <pre>
{`# These endpoints require an admin API key or an admin dashboard session.
# Switch to flaky mode (30% failure rate)
curl -X POST http://localhost:8090/v1/gateway/scenarios \\
  -u "$ADMIN_KEY_ID:$ADMIN_KEY_SECRET" \\
  -H 'Content-Type: application/json' \\
  -d '{"mode":"flaky","failure_rate":0.30,"delay_ms":0}'

# Switch back to success mode
curl -X POST http://localhost:8090/v1/gateway/scenarios \\
  -u "$ADMIN_KEY_ID:$ADMIN_KEY_SECRET" \\
  -d '{"mode":"success"}'

# Check active scenario
curl -u "$ADMIN_KEY_ID:$ADMIN_KEY_SECRET" \\
  http://localhost:8090/v1/gateway/scenarios/active`}
        </pre>
      </div>
    </section>
  );
}
