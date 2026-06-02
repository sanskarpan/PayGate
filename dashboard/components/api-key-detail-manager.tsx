"use client";

import { useState } from "react";

import IPAllowlistManager from "./ip-allowlist-manager";
import { RouteIcon } from "./system-icons";
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
      <div className="ops-grid">
        <div className="hero-card has-glyph">
          <div className="page-glyph">
            <div className="page-glyph-badge">
              <RouteIcon name="api-keys" size={40} />
            </div>
            <div className="page-glyph-label">
              <span>Credential file</span>
              <strong>Rotation lane</strong>
            </div>
          </div>
          <div className="eyebrow">Credential Detail</div>
          <h1>{item.id}</h1>
          <p className="lede">
            Review lifecycle metadata, rotate the credential, and manage its source-IP policy
            without dropping out of the security rail.
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

        <div className="detail-card">
          <div className="eyebrow">Security Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Last used</span>
              <strong>{item.last_used_at ? formatTime(item.last_used_at) : "Never"}</strong>
              <p>Latest observed use for this merchant credential.</p>
            </div>
            <div className="status-block">
              <span>Allowlist</span>
              <strong>{item.allowed_ips?.length ?? 0}</strong>
              <p>Source-IP constraints currently attached to this key.</p>
            </div>
            <div className="status-block">
              <span>Revocation</span>
              <strong>{item.revoked_at ? "Revoked" : "Live"}</strong>
              <p>Credential lifecycle state at the moment this page was loaded.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="workbench-grid">
        <div className="detail-card">
          <div className="section-head">
            <div>
              <div className="eyebrow">Lifecycle Record</div>
              <h2>Credential chronology</h2>
            </div>
          </div>
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
          <div className="section-head">
            <div>
              <div className="eyebrow">Rotation Rail</div>
              <h2>Replace credential</h2>
            </div>
          </div>
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
