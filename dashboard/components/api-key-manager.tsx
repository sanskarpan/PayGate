"use client";

import { useDeferredValue, useMemo, useState } from "react";

import { formatTime, type APIKeyItem } from "../lib/types";

type CreateKeyResponse = {
  key_id: string;
  key_secret: string;
  mode: string;
  scope: string;
};

export default function APIKeyManager({
  apiBaseUrl,
  initialItems,
}: {
  apiBaseUrl: string;
  initialItems: APIKeyItem[];
}) {
  const [items, setItems] = useState(initialItems);
  const [mode, setMode] = useState("test");
  const [scope, setScope] = useState("write");
  const [statusFilter, setStatusFilter] = useState<"all" | "active" | "revoked">("all");
  const [query, setQuery] = useState("");
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [created, setCreated] = useState<CreateKeyResponse | null>(null);
  const deferredQuery = useDeferredValue(query);

  const activeCount = useMemo(
    () => items.filter((item) => item.status === "active").length,
    [items],
  );

  const visibleItems = useMemo(() => {
    const lowered = deferredQuery.trim().toLowerCase();
    return items.filter((item) => {
      if (statusFilter !== "all" && item.status !== statusFilter) {
        return false;
      }
      if (!lowered) {
        return true;
      }
      return [item.id, item.mode, item.scope, item.status].some((value) =>
        value.toLowerCase().includes(lowered),
      );
    });
  }, [items, statusFilter, deferredQuery]);

  async function createKey() {
    setPending(true);
    setMessage("");
    setCreated(null);
    try {
      const response = await fetch(`${apiBaseUrl}/v1/merchants/me/api-keys`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ mode, scope }),
      });
      const data = await response.json();
      if (!response.ok) {
        setMessage(data?.error?.description || "Key creation failed");
        return;
      }
      const createdKey = data as CreateKeyResponse;
      setCreated(createdKey);
      setItems((current) => [
        {
          id: createdKey.key_id,
          mode: createdKey.mode,
          scope: createdKey.scope,
          status: "active",
          last_used_at: 0,
          revoked_at: 0,
          created_at: Math.floor(Date.now() / 1000),
        },
        ...current,
      ]);
    } finally {
      setPending(false);
    }
  }

  async function revokeKey(id: string) {
    setPending(true);
    setMessage("");
    try {
      const response = await fetch(`${apiBaseUrl}/v1/merchants/me/api-keys/${id}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (!response.ok) {
        const data = await response.json();
        setMessage(data?.error?.description || "Key revoke failed");
        return;
      }
      setItems((current) =>
        current.map((item) =>
          item.id === id
            ? { ...item, status: "revoked", revoked_at: Math.floor(Date.now() / 1000) }
            : item,
        ),
      );
    } finally {
      setPending(false);
    }
  }

  return (
    <section className="stack fade-up">
      <div className="workbench-grid">
        <div className="hero-card">
          <div className="eyebrow">Credential Control</div>
          <h1>Issue and constrain keys.</h1>
          <p className="lede">
            Manage live credentials for integrations with immediate visibility into mode, scope,
            revoke state, and allowlist posture.
          </p>
          <div className="metric-strip">
            <div className="metric-chip">
              <span className="metric-chip-label">Total keys</span>
              <strong>{items.length}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Active</span>
              <strong>{activeCount}</strong>
            </div>
          </div>
        </div>

        <div className="detail-card">
          <div className="section-head">
            <div>
              <div className="eyebrow">Create Credential</div>
              <h2>New key issuance</h2>
            </div>
          </div>
          <div className="stack" style={{ marginTop: "16px" }}>
            <div className="control-form-grid">
              <label>
                Mode
                <select value={mode} onChange={(event) => setMode(event.target.value)}>
                  <option value="test">test</option>
                  <option value="live">live</option>
                </select>
              </label>
              <label>
                Scope
                <select value={scope} onChange={(event) => setScope(event.target.value)}>
                  <option value="read">read</option>
                  <option value="write">write</option>
                  <option value="admin">admin</option>
                </select>
              </label>
            </div>
            <div className="hero-actions">
              <button className="primary-button" type="button" disabled={pending} onClick={createKey}>
                {pending ? "Working..." : "Create key"}
              </button>
            </div>
            {message ? <p className="notice error">{message}</p> : null}
            {created ? (
              <div className="secret-card">
                <div className="eyebrow">Secret Shown Once</div>
                <code>{created.key_id}</code>
                <code>{created.key_secret}</code>
              </div>
            ) : null}
          </div>
        </div>
      </div>

      <div className="detail-card">
        <div className="section-head">
          <div>
            <div className="eyebrow">Query Controls</div>
            <h2>Filter inventory</h2>
          </div>
        </div>
        <div className="stack" style={{ marginTop: "16px" }}>
          <div className="control-form-grid">
            <label style={{ minWidth: "220px" }}>
              Search
              <input
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Filter by key id, mode, scope, or status"
                type="text"
                value={query}
              />
            </label>
          </div>
          <div className="filter-bar" role="tablist" aria-label="API key status filter">
            {[
              ["all", `All (${items.length})`],
              ["active", `Active (${activeCount})`],
              ["revoked", `Revoked (${items.length - activeCount})`],
            ].map(([value, label]) => (
              <button
                aria-selected={statusFilter === value}
                className={`filter-pill${statusFilter === value ? " active" : ""}`}
                key={value}
                onClick={() => setStatusFilter(value as "all" | "active" | "revoked")}
                role="tab"
                type="button"
              >
                {label}
              </button>
            ))}
          </div>
        </div>
      </div>

      <div className="list-card">
        <div className="section-head">
          <div>
            <div className="eyebrow">Key Inventory</div>
            <h2>Issued credentials</h2>
          </div>
        </div>
        {visibleItems.length === 0 ? (
          <div className="empty-state">
            <strong>{items.length === 0 ? "No keys issued yet" : "No keys match this filter"}</strong>
            <span className="muted">
              {items.length === 0
                ? "Create read, write, or admin credentials for merchant integrations."
                : "Change the filter or search text to broaden the result set."}
            </span>
          </div>
        ) : (
          visibleItems.map((item) => (
            <article className="list-row" key={item.id}>
              <div className="entity-primary">
                <div className="row-title">{item.id}</div>
                <div className="row-meta">
                  <span className="badge-neutral">{item.mode}</span>
                  <span className="badge-info">{item.scope}</span>
                  <span className={item.status === "active" ? "badge-success" : "badge-warning"}>
                    {item.status}
                  </span>
                  {item.allowed_ips?.length ? <span>{item.allowed_ips.length} allowlisted IP entries</span> : null}
                  <span>{item.last_used_at ? `Last used ${formatTime(item.last_used_at)}` : "Never used"}</span>
                </div>
              </div>
              <div className="row-actions">
                <a className="ghost-button" href={`/api-keys/${item.id}`}>
                  IP Allowlist
                </a>
                <button
                  className="ghost-button"
                  disabled={pending || item.status !== "active"}
                  onClick={() => revokeKey(item.id)}
                  type="button"
                >
                  Revoke
                </button>
              </div>
            </article>
          ))
        )}
      </div>
    </section>
  );
}
