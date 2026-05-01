"use client";

import { useState } from "react";

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
}: {
  keyId: string;
  apiBaseUrl: string;
}) {
  const [ips, setIps] = useState<string[]>([]);
  const [input, setInput] = useState("");
  const [inputError, setInputError] = useState("");
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [messageType, setMessageType] = useState<"error" | "success">("error");

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
      <div className="hero-card">
        <div className="eyebrow">Security</div>
        <h1>IP Allowlist</h1>
        <p className="lede">
          Restrict this API key to requests originating from specific IP
          addresses or CIDR ranges. Leave the list empty to allow all IPs.
        </p>
        <p className="row-meta">
          <span>Key ID:</span>
          <code>{keyId}</code>
        </p>
      </div>

      <div className="list-card">
        <div className="list-row">
          <div className="inline-form" style={{ flex: 1 }}>
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
            <button
              className="action-button"
              type="button"
              disabled={pending}
              onClick={addIP}
            >
              Add
            </button>
          </div>
        </div>
        {inputError ? <p className="notice error">{inputError}</p> : null}
      </div>

      <div className="list-card">
        {ips.length === 0 ? (
          <p className="muted">No IPs added — all source addresses are currently allowed.</p>
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

      <div className="list-card">
        <div className="list-row">
          <button
            className="primary-button"
            type="button"
            disabled={pending}
            onClick={save}
          >
            {pending ? "Saving..." : "Save Allowlist"}
          </button>
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
