"use client";

import { useState } from "react";

import IPAllowlistManager from "./ip-allowlist-manager";
import { formatTime, type APIKeyItem } from "../lib/types";

type RotateKeyResponse = {
  key_id: string;
  key_secret: string;
  mode: string;
  scope: string;
};

export default function APIKeyDetailManager({
  apiBaseUrl,
  item,
}: {
  apiBaseUrl: string;
  item: APIKeyItem;
}) {
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [rotated, setRotated] = useState<RotateKeyResponse | null>(null);

  async function rotateKey() {
    setPending(true);
    setMessage("");
    setRotated(null);
    try {
      const response = await fetch(`${apiBaseUrl}/v1/merchants/me/api-keys/${item.id}/rotate`, {
        method: "POST",
        credentials: "include",
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        setMessage((data as { error?: { description?: string } }).error?.description ?? "Key rotation failed");
        return;
      }
      setRotated(data as RotateKeyResponse);
    } catch {
      setMessage("Network error while rotating the key.");
    } finally {
      setPending(false);
    }
  }

  return (
    <section className="stack fade-up">
      <div className="hero-card">
        <div className="eyebrow">Credential Detail</div>
        <h1>{item.id}</h1>
        <p className="lede">
          Review lifecycle metadata, rotate the credential, and manage its source-IP policy.
        </p>
        <div className="metric-strip">
          <div className="metric-chip">
            <span className="metric-chip-label">Mode</span>
            <strong>{item.mode}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Scope</span>
            <strong>{item.scope}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Status</span>
            <strong>{item.status}</strong>
          </div>
        </div>
      </div>

      <div className="detail-grid">
        <div className="detail-card">
          <h2>Lifecycle</h2>
          <dl className="detail-list">
            <div>
              <dt>Created</dt>
              <dd>{formatTime(item.created_at)}</dd>
            </div>
            <div>
              <dt>Last used</dt>
              <dd>{item.last_used_at ? formatTime(item.last_used_at) : "Not yet used"}</dd>
            </div>
            <div>
              <dt>Revoked at</dt>
              <dd>{item.revoked_at ? formatTime(item.revoked_at) : "Active"}</dd>
            </div>
            <div>
              <dt>Allowed IP entries</dt>
              <dd>{item.allowed_ips?.length ?? 0}</dd>
            </div>
          </dl>
        </div>

        <div className="detail-card">
          <h2>Rotate key</h2>
          <p className="muted">
            Rotation revokes the current credential and issues a replacement with the same mode and scope.
          </p>
          <div className="hero-actions">
            <button
              className="primary-button"
              disabled={pending || item.status !== "active"}
              onClick={rotateKey}
              type="button"
            >
              {pending ? "Rotating..." : "Rotate Credential"}
            </button>
          </div>
          {message ? <p className="notice error">{message}</p> : null}
          {rotated ? (
            <div className="secret-card" style={{ marginTop: "16px" }}>
              <div className="eyebrow">Replacement Secret Shown Once</div>
              <code>{rotated.key_id}</code>
              <code>{rotated.key_secret}</code>
            </div>
          ) : null}
        </div>
      </div>

      <IPAllowlistManager
        apiBaseUrl={apiBaseUrl}
        initialIPs={item.allowed_ips ?? []}
        keyId={item.id}
      />
    </section>
  );
}
