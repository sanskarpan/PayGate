"use client";

import { useMemo, useState } from "react";

import type { APIKeyItem } from "../lib/types";

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
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [created, setCreated] = useState<CreateKeyResponse | null>(null);

  const activeCount = useMemo(
    () => items.filter((item) => item.status === "active").length,
    [items],
  );

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
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Access Surface</div>
        <h1>API Keys</h1>
        <p className="lede">
          Manage live credentials for integrations. Active keys: {activeCount}.
        </p>
        <div className="inline-form">
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
          <button className="primary-button" type="button" disabled={pending} onClick={createKey}>
            {pending ? "Working..." : "Create Key"}
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

      <div className="list-card">
        {items.length === 0 ? (
          <p className="muted">No keys issued yet.</p>
        ) : (
          items.map((item) => (
            <article className="list-row" key={item.id}>
              <div>
                <div className="row-title">{item.id}</div>
                <div className="row-meta">
                  <span>{item.mode}</span>
                  <span>{item.scope}</span>
                  <span>{item.status}</span>
                </div>
              </div>
              <button
                className="ghost-button"
                disabled={pending || item.status !== "active"}
                onClick={() => revokeKey(item.id)}
                type="button"
              >
                Revoke
              </button>
            </article>
          ))
        )}
      </div>
    </section>
  );
}
