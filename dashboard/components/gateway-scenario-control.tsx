"use client";

import { useState } from "react";

import type { GatewayScenario } from "../lib/types";

const presets = [
  { mode: "success", label: "Happy Path", desc: "All authorizations succeed", failureRate: 0, delayMs: 0, declineCode: "CARD_DECLINED" },
  { mode: "slow", label: "Slow Gateway", desc: "3 second delay before approval", failureRate: 0, delayMs: 3000, declineCode: "CARD_DECLINED" },
  { mode: "flaky", label: "Flaky 30%", desc: "Randomized failure conditions", failureRate: 0.3, delayMs: 0, declineCode: "BANK_UNAVAILABLE" },
  { mode: "decline", label: "Decline All", desc: "Every authorization declines", failureRate: 0, delayMs: 0, declineCode: "CARD_DECLINED" },
  { mode: "timeout", label: "Timeout", desc: "No gateway response", failureRate: 0, delayMs: 0, declineCode: "GATEWAY_TIMEOUT" },
  { mode: "late_callback", label: "Late Callback", desc: "Approval after long callback lag", failureRate: 0, delayMs: 4500, declineCode: "CARD_DECLINED" },
] as const;

export default function GatewayScenarioControl({
  apiBaseUrl,
  initialActive,
  initialItems,
}: {
  apiBaseUrl: string;
  initialActive: GatewayScenario | null;
  initialItems: GatewayScenario[];
}) {
  const [active, setActive] = useState<GatewayScenario | null>(initialActive);
  const [items, setItems] = useState(initialItems);
  const [mode, setMode] = useState(initialActive?.mode ?? "success");
  const [delayMs, setDelayMs] = useState(initialActive?.delay_ms ?? 0);
  const [failureRate, setFailureRate] = useState(initialActive?.failure_rate ?? 0);
  const [declineCode, setDeclineCode] = useState(initialActive?.decline_code ?? "CARD_DECLINED");
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");

  async function applyScenario(payload: {
    mode: string;
    delay_ms?: number;
    failure_rate?: number;
    decline_code?: string;
  }) {
    setPending(true);
    setMessage("");
    try {
      const response = await fetch(`${apiBaseUrl}/v1/gateway/scenarios`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        setMessage((data as { error?: { description?: string } }).error?.description ?? "Failed to update gateway scenario");
        return;
      }
      const scenario = data as GatewayScenario;
      setActive(scenario);
      setItems((current) => [scenario, ...current.filter((item) => item.id !== scenario.id)]);
      setMode(scenario.mode);
      setDelayMs(scenario.delay_ms);
      setFailureRate(scenario.failure_rate);
      setDeclineCode(scenario.decline_code);
      setMessage("Gateway scenario updated.");
    } catch {
      setMessage("Network error while updating gateway scenario.");
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="stack">
      <div className="detail-card">
        <div className="section-head">
          <div>
            <h2>Inline scenario control</h2>
            <div className="section-kicker">Apply known failure modes without leaving the dashboard</div>
          </div>
        </div>
        <div className="triple-grid" style={{ marginTop: "16px" }}>
          {presets.map((preset) => (
            <button
              className={`spotlight-card${active?.mode === preset.mode ? " active-card" : ""}`}
              disabled={pending}
              key={preset.mode}
              onClick={() =>
                applyScenario({
                  mode: preset.mode,
                  delay_ms: preset.delayMs,
                  failure_rate: preset.failureRate,
                  decline_code: preset.declineCode,
                })
              }
              style={{ textAlign: "left", cursor: "pointer" }}
              type="button"
            >
              <div className="row-title">{preset.label}</div>
              <div className="row-meta">
                <span>{preset.mode}</span>
              </div>
              <div className="muted">{preset.desc}</div>
            </button>
          ))}
        </div>
      </div>

      <div className="detail-card">
        <div className="section-head">
          <div>
            <h2>Custom scenario</h2>
            <div className="section-kicker">Tune latency, failure rate, and decline behavior</div>
          </div>
        </div>
        <div className="inline-form" style={{ marginTop: "16px" }}>
          <label>
            Mode
            <select onChange={(event) => setMode(event.target.value)} value={mode}>
              <option value="success">success</option>
              <option value="slow">slow</option>
              <option value="flaky">flaky</option>
              <option value="decline">decline</option>
              <option value="timeout">timeout</option>
              <option value="late_callback">late_callback</option>
            </select>
          </label>
          <label>
            Delay (ms)
            <input min={0} onChange={(event) => setDelayMs(Number(event.target.value) || 0)} type="number" value={delayMs} />
          </label>
          <label>
            Failure Rate
            <input max={1} min={0} onChange={(event) => setFailureRate(Number(event.target.value) || 0)} step="0.05" type="number" value={failureRate} />
          </label>
          <label>
            Decline Code
            <input onChange={(event) => setDeclineCode(event.target.value)} type="text" value={declineCode} />
          </label>
        </div>
        <div className="hero-actions">
          <button
            className="primary-button"
            disabled={pending}
            onClick={() =>
              applyScenario({
                mode,
                delay_ms: delayMs,
                failure_rate: failureRate,
                decline_code: declineCode,
              })
            }
            type="button"
          >
            {pending ? "Applying..." : "Apply Custom Scenario"}
          </button>
          {message ? <span className="micro-copy">{message}</span> : null}
        </div>
      </div>

      <div className="list-card">
        <div className="section-head">
          <div>
            <h2>Scenario history</h2>
            <div className="section-kicker">Most recent control changes</div>
          </div>
        </div>
        {items.length === 0 ? (
          <div className="empty-state">
            <strong>No scenarios configured</strong>
            <span className="muted">Default behavior remains the success path until a scenario is applied.</span>
          </div>
        ) : (
          items.map((sc) => (
            <div className="list-row" key={sc.id}>
              <div className="entity-primary">
                <div className="row-title">{sc.merchant_id || "Global"}</div>
                <div className="row-meta">
                  <span>{sc.mode}</span>
                  {sc.failure_rate > 0 ? <span>{Math.round(sc.failure_rate * 100)}% failure</span> : null}
                  {sc.delay_ms > 0 ? <span>{sc.delay_ms}ms delay</span> : null}
                  {sc.active ? <span className="badge-success">active</span> : null}
                  <span>{new Date(sc.created_at * 1000).toLocaleString("en-IN", { dateStyle: "medium", timeStyle: "short" })}</span>
                </div>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
