"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";

import { formatMoney, formatTime, type PayoutItem } from "../lib/types";

type SavedView = {
  name: string;
  query: string;
  approval: string;
  status: string;
};

const storageKey = "paygate:payout-saved-views";

function statusBadge(status: string) {
  switch (status) {
    case "completed":
      return "badge-success";
    case "failed":
    case "reversed":
      return "badge-error";
    case "processing":
    case "returned":
      return "badge-warning";
    default:
      return "badge-neutral";
  }
}

export default function PayoutApprovalManager({ items }: { items: PayoutItem[] }) {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [approval, setApproval] = useState("pending");
  const [status, setStatus] = useState("all");
  const [selected, setSelected] = useState<string[]>([]);
  const [savedViews, setSavedViews] = useState<SavedView[]>([]);
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");

  useEffect(() => {
    try {
      const raw = window.localStorage.getItem(storageKey);
      if (!raw) return;
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        setSavedViews(parsed);
      }
    } catch {
      return;
    }
  }, []);

  useEffect(() => {
    window.localStorage.setItem(storageKey, JSON.stringify(savedViews));
  }, [savedViews]);

  const visibleItems = useMemo(() => {
    const lowered = query.trim().toLowerCase();
    return items.filter((item) => {
      if (approval !== "all" && (item.approval_status || "none") !== approval) return false;
      if (status !== "all" && item.status !== status) return false;
      if (!lowered) return true;
      return [item.id, item.settlement_id, item.beneficiary_id, item.bank_reference, item.failure_reason]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(lowered));
    });
  }, [approval, items, query, status]);

  async function run(path: string) {
    const response = await fetch(`/api/proxy${path}`, { method: "POST" });
    if (!response.ok) {
      const data = await response.json().catch(() => ({}));
      throw new Error((data as { error?: { description?: string } }).error?.description ?? "Payout action failed");
    }
  }

  async function bulk(decision: "approve" | "reject") {
    setPending(true);
    setMessage("");
    try {
      for (const id of selected) {
        await run(`/v1/payouts/${id}/${decision}`);
      }
      setSelected([]);
      setMessage(`${decision === "approve" ? "Approved" : "Rejected"} ${selected.length} payout(s).`);
      router.refresh();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Bulk payout action failed");
    } finally {
      setPending(false);
    }
  }

  function saveView() {
    const name = window.prompt("Saved view name");
    if (!name) return;
    setSavedViews((current) => [...current, { name, query, approval, status }]);
  }

  function applyView(view: SavedView) {
    setQuery(view.query);
    setApproval(view.approval);
    setStatus(view.status);
  }

  function toggle(id: string) {
    setSelected((current) => (current.includes(id) ? current.filter((item) => item !== id) : [...current, id]));
  }

  return (
    <div className="stack">
      <div className="workbench-grid">
        <div className="detail-card">
          <div className="section-head">
            <div>
              <div className="eyebrow">Approval Filters</div>
              <h2>Treasury workbench</h2>
              <div className="section-kicker">Filter the queue, preserve views, and stage bulk approval or rejection.</div>
            </div>
          </div>
          <div className="stack" style={{ marginTop: "16px" }}>
            <div className="control-form-grid">
              <label style={{ minWidth: "220px" }}>
                Search
                <input value={query} onChange={(event) => setQuery(event.target.value)} type="text" placeholder="payout, settlement, beneficiary" />
              </label>
              <label>
                Approval
                <select value={approval} onChange={(event) => setApproval(event.target.value)}>
                  <option value="pending">pending</option>
                  <option value="approved">approved</option>
                  <option value="rejected">rejected</option>
                  <option value="all">all</option>
                </select>
              </label>
              <label>
                Status
                <select value={status} onChange={(event) => setStatus(event.target.value)}>
                  <option value="all">all</option>
                  <option value="pending">pending</option>
                  <option value="processing">processing</option>
                  <option value="completed">completed</option>
                  <option value="failed">failed</option>
                  <option value="returned">returned</option>
                </select>
              </label>
            </div>
            <div className="hero-actions">
              <button className="ghost-button" type="button" onClick={saveView}>Save current view</button>
              {savedViews.map((view) => (
                <button key={view.name} className="filter-pill" type="button" onClick={() => applyView(view)}>
                  {view.name}
                </button>
              ))}
            </div>
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Selection Rail</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Visible</span>
              <strong>{visibleItems.length}</strong>
              <p>Payout records currently matching the selected treasury view.</p>
            </div>
            <div className="status-block">
              <span>Selected</span>
              <strong>{selected.length}</strong>
              <p>Rows presently staged for approval or rejection.</p>
            </div>
            <div className="status-block">
              <span>Saved views</span>
              <strong>{savedViews.length}</strong>
              <p>Reusable query presets stored locally for repeat treasury work.</p>
            </div>
          </div>
          <div className="hero-actions">
            <button className="ghost-button" type="button" disabled={pending || selected.length === 0} onClick={() => bulk("approve")}>Approve selected</button>
            <button className="ghost-button" type="button" disabled={pending || selected.length === 0} onClick={() => bulk("reject")}>Reject selected</button>
          </div>
          {message ? <p className={`notice ${message.includes("failed") || message.includes("error") ? "error" : "success"}`}>{message}</p> : null}
        </div>
      </div>

      <div className="list-card">
        <div className="section-head">
          <div>
            <div className="eyebrow">Approval Queue</div>
            <h2>Payout ledger</h2>
          </div>
        </div>
        {visibleItems.length === 0 ? (
          <div className="empty-state">
            <strong>No payouts match this view</strong>
            <span className="muted">Adjust approval or status filters to expand the queue.</span>
          </div>
        ) : (
          visibleItems.map((payout) => (
            <article className="list-row" key={payout.id}>
              <div className="row-actions">
                <input
                  type="checkbox"
                  checked={selected.includes(payout.id)}
                  onChange={() => toggle(payout.id)}
                  aria-label={`Select ${payout.id}`}
                  disabled={payout.approval_status !== "pending"}
                />
              </div>
              <div style={{ flex: 1 }}>
                <div className="row-title">{payout.id}</div>
                <div className="row-meta">
                  <span className={statusBadge(payout.status)}>{payout.status}</span>
                  {payout.approval_status ? <span>{payout.approval_status}</span> : null}
                  <span>Settlement: {payout.settlement_id}</span>
                  {payout.beneficiary_id ? <span>Beneficiary: {payout.beneficiary_id}</span> : null}
                  {payout.bank_reference ? <span>Ref: {payout.bank_reference}</span> : null}
                  <span>{formatTime(payout.created_at)}</span>
                </div>
                {payout.failure_reason ? <div className="muted">{payout.failure_reason}</div> : null}
              </div>
              <div className="amount-pill">{formatMoney(payout.amount, payout.currency)}</div>
            </article>
          ))
        )}
      </div>
    </div>
  );
}
