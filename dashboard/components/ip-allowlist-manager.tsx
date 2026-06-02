"use client";

import { useState } from "react";

import { RouteIcon } from "./system-icons";

// Validates a plain IPv4/IPv6 address or a CIDR range.
// Examples: "192.168.1.1", "10.0.0.0/8", "::1", "2001:db8::/32"
function isValidIPOrCIDR(value: string): boolean {
  const cidrIPv4 = /^(\d{1,3}\.){3}\d{1,3}(\/([0-9]|[1-2]\d|3[0-2]))?$/;
  const cidrIPv6 = /^[0-9a-fA-F:]+(%[^\s/]+)?(\/([0-9]|[1-9]\d|1[01]\d|12[0-8]))?$/;

  const trimmed = value.trim();
  if (!trimmed) return false;

  if (trimmed.includes(":")) {
    // IPv6
    return cidrIPv6.test(trimmed);
  }
  // IPv4
  if (!cidrIPv4.test(trimmed)) return false;
  const parts = trimmed.split("/")[0].split(".");
  return parts.every((p) => Number(p) >= 0 && Number(p) <= 255);
}

export default function IPAllowlistManager({
  keyId,
  apiBaseUrl,
  initialIPs,
}: {
  keyId: string;
  apiBaseUrl: string;
  initialIPs: string[];
}) {
  const [ips, setIps] = useState<string[]>(initialIPs);
  const [input, setInput] = useState("");
  const [inputError, setInputError] = useState("");
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [messageType, setMessageType] = useState<"error" | "success">("error");
  const dirty = JSON.stringify(ips) !== JSON.stringify(initialIPs);

  function addIP() {
    const trimmed = input.trim();
    if (!trimmed) return;
    if (!isValidIPOrCIDR(trimmed)) {
      setInputError("Enter a valid IP address or CIDR range (e.g. 10.0.0.1 or 192.168.0.0/24).");
      return;
    }
    if (ips.includes(trimmed)) {
      setInputError("This IP is already in the list.");
      return;
    }
    setInputError("");
    setIps((current) => [...current, trimmed]);
    setInput("");
  }

  function removeIP(ip: string) {
    setIps((current) => current.filter((item) => item !== ip));
  }

  function reset() {
    setIps(initialIPs);
    setInput("");
    setInputError("");
    setMessage("");
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter") {
      e.preventDefault();
      addIP();
    }
  }

  async function save() {
    setPending(true);
    setMessage("");
    try {
      const response = await fetch(
        `${apiBaseUrl}/v1/merchants/me/api-keys/${keyId}/allowed-ips`,
        {
          method: "PUT",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ allowed_ips: ips }),
        },
      );
      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        const desc =
          (data as { error?: { description?: string } }).error?.description ??
          `Request failed (${response.status})`;
        setMessageType("error");
        setMessage(desc);
        return;
      }
      setMessageType("success");
      setMessage("IP allowlist saved successfully.");
    } catch {
      setMessageType("error");
      setMessage("Network error — changes not saved.");
    } finally {
      setPending(false);
    }
  }

  return (
    <section className="stack">
      <div className="ops-grid">
        <div className="hero-card has-glyph">
          <div className="page-glyph">
            <div className="page-glyph-badge">
              <RouteIcon name="gateway" size={40} />
            </div>
            <div className="page-glyph-label">
              <span>Source list</span>
              <strong>Boundary rail</strong>
            </div>
          </div>
          <div className="eyebrow">Source Boundary</div>
          <h1>IP allowlist control.</h1>
          <p className="lede">
            Restrict this API key to traffic from specific IP addresses or CIDR ranges. Leave the
            list empty only when open ingress is intentional.
          </p>
          <div className="metric-strip">
            <div className="metric-chip">
              <span className="metric-chip-label">Key</span>
              <code>{keyId}</code>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Entries</span>
              <strong>{ips.length}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Policy</span>
              <strong>{ips.length === 0 ? "Open" : "Restricted"}</strong>
            </div>
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Boundary Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Exposure</span>
              <strong>{ips.length === 0 ? "Open" : "Narrowed"}</strong>
              <p>Whether this credential is currently limited to explicit source origins.</p>
            </div>
            <div className="status-block">
              <span>Unsaved drift</span>
              <strong>{dirty ? "Present" : "None"}</strong>
              <p>Edits made since the page was opened from the server snapshot.</p>
            </div>
            <div className="status-block">
              <span>Address forms</span>
              <strong>IPv4 / IPv6</strong>
              <p>Accepts individual addresses and CIDR ranges across both families.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="detail-card">
        <div className="section-head">
          <div>
            <div className="eyebrow">Add Origin</div>
            <h2>Ingress constraint</h2>
          </div>
        </div>
        <div className="stack" style={{ marginTop: "16px" }}>
          <div className="control-form-grid">
            <label style={{ gridColumn: "1 / -1" }}>
              IP or CIDR
              <input
                className="input"
                type="text"
                value={input}
                placeholder="e.g. 203.0.113.0/24 or 198.51.100.42"
                onChange={(e) => {
                  setInput(e.target.value);
                  setInputError("");
                }}
                onKeyDown={handleKeyDown}
                aria-label="IP address or CIDR"
              />
            </label>
          </div>
          <div className="hero-actions">
            <button
              className="action-button"
              type="button"
              disabled={pending}
              onClick={addIP}
            >
              Add origin
            </button>
            <span className="micro-copy">Examples: 203.0.113.8, 198.51.100.0/24, 2001:db8::/32</span>
          </div>
          {inputError ? <p className="notice error">{inputError}</p> : null}
        </div>
      </div>

      <div className="list-card">
        <div className="section-head">
          <div>
            <div className="eyebrow">Allowed Origins</div>
            <h2>Current entries</h2>
          </div>
        </div>
        {ips.length === 0 ? (
          <div className="empty-state">
            <strong>No IP restrictions</strong>
            <span className="muted">All source addresses are currently allowed for this key.</span>
          </div>
        ) : (
          ips.map((ip) => (
            <div className="list-row" key={ip}>
              <div>
                <code className="row-title">{ip}</code>
              </div>
              <button
                className="ghost-button"
                type="button"
                disabled={pending}
                onClick={() => removeIP(ip)}
              >
                Remove
              </button>
            </div>
          ))
        )}
      </div>

      <div className="detail-card">
        <div className="section-head">
          <div>
            <div className="eyebrow">Persist Changes</div>
            <h2>Apply boundary</h2>
          </div>
        </div>
        <div className="list-row" style={{ marginTop: "16px" }}>
          <div className="row-actions">
            <button
              className="primary-button"
              type="button"
              disabled={pending || !dirty}
              onClick={save}
            >
              {pending ? "Saving..." : "Save Allowlist"}
            </button>
            <button
              className="ghost-button"
              type="button"
              disabled={pending || !dirty}
              onClick={reset}
            >
              Reset Changes
            </button>
          </div>
          <div className="micro-copy">
            {dirty ? "Unsaved changes" : "Saved state matches the server snapshot used to open this page"}
          </div>
        </div>
        {message ? (
          <p className={`notice ${messageType === "success" ? "success" : "error"}`}>
            {message}
          </p>
        ) : null}
      </div>
    </section>
  );
}
