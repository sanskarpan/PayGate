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
      <div className="ops-grid">
        <div className="hero-card">
          <div className="eyebrow">Gateway Rail</div>
          <h1>Failure-path command deck.</h1>
          <p className="lede">
            Shape gateway behavior for decline drills, callback disorder, timeout simulation, and
            latency pressure without leaving the operator surface.
          </p>
          <div className="metric-strip">
            <div className="metric-chip">
              <span className="metric-chip-label">Active mode</span>
              <strong>{active?.mode ?? "success"}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Scenario count</span>
              <strong>{scenarios.count}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Pressure lane</span>
              <strong>{active?.delay_ms ? `${active.delay_ms}ms` : "Nominal"}</strong>
            </div>
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Global state</span>
              <strong>{active ? active.mode : "success"}</strong>
              <p>{active ? modeDescription(active.mode, active.failure_rate, active.delay_ms) : "Default nominal path is active."}</p>
            </div>
            <div className="status-block">
              <span>Decline code</span>
              <strong>{active?.decline_code ?? "CARD_DECLINED"}</strong>
              <p>Primary rejection reason applied when the simulator is in a decline lane.</p>
            </div>
            <div className="status-block">
              <span>Callback lag</span>
              <strong>{active?.delay_ms ? `${active.delay_ms}ms` : "Instant"}</strong>
              <p>Delay currently shaping callback or authorization response timing.</p>
            </div>
          </div>
        </div>
      </div>

      <GatewayScenarioControl
        apiBaseUrl={getApiBaseUrl()}
        initialActive={active}
        initialItems={scenarios.items}
      />

      <div className="detail-card">
        <div className="section-head">
          <div>
            <div className="eyebrow">Operator API</div>
            <h2>CLI examples</h2>
          </div>
        </div>
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
