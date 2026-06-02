import APIKeyManager from "../../components/api-key-manager";
import { RouteIcon } from "../../components/system-icons";
import { getAPIKeys, getApiBaseUrl, requireViewer } from "../../lib/api";

export default async function APIKeysPage() {
  const viewer = await requireViewer();
  const keys = await getAPIKeys();

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card has-glyph">
          <div className="page-glyph">
            <div className="page-glyph-badge">
              <RouteIcon name="api-keys" size={40} />
            </div>
            <div className="page-glyph-label">
              <span>Credential lattice</span>
              <strong>Key boundary</strong>
            </div>
          </div>
          <div className="eyebrow">Credential Fabric</div>
          <h1>API credential control.</h1>
          <p className="lede">
            Issue, revoke, and inspect integration keys for {viewer.merchant_id} with mode, scope,
            and allowlist posture visible in one lane.
          </p>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Security Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Total keys</span>
              <strong>{keys.items.length}</strong>
              <p>Credential inventory presently attached to this merchant.</p>
            </div>
            <div className="status-block">
              <span>Active</span>
              <strong>{keys.items.filter((item) => item.status === "active").length}</strong>
              <p>Keys currently usable by downstream integrations.</p>
            </div>
            <div className="status-block">
              <span>Allowlisted</span>
              <strong>{keys.items.filter((item) => item.allowed_ips?.length).length}</strong>
              <p>Credentials already narrowed by explicit IP boundary.</p>
            </div>
          </div>
        </div>
      </div>

      <APIKeyManager apiBaseUrl={getApiBaseUrl()} initialItems={keys.items} />
    </section>
  );
}
