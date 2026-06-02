"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";

import { formatTime, type RiskEventItem } from "../lib/types";

type SavedView = {
  name: string;
  query: string;
  status: string;
  action: string;
  assigned: string;
};

const storageKey = "paygate:risk-saved-views";

function actionBadge(action: string) {
  if (action === "block") return "badge-error";
  if (action === "hold") return "badge-warning";
  return "badge-success";
}

export default function RiskQueueManager({ initialItems }: { initialItems: RiskEventItem[] }) {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("open");
  const [action, setAction] = useState("all");
  const [assigned, setAssigned] = useState("all");
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
    return initialItems.filter((item) => {
      if (status === "open" && item.resolved) return false;
      if (status === "resolved" && !item.resolved) return false;
      if (action !== "all" && item.action !== action) return false;
      if (assigned === "assigned" && !item.assigned_to) return false;
      if (assigned === "unassigned" && item.assigned_to) return false;
      if (!lowered) return true;
      return [
        item.id,
        item.payment_id,
        item.review_status,
        item.manual_decision,
        item.assigned_to,
        item.triggered_rules.join(" "),
      ]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(lowered));
    });
  }, [action, assigned, initialItems, query, status]);

  async function run(path: string, body?: unknown) {
    const response = await fetch(`/api/proxy${path}`, {
      method: "POST",
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!response.ok) {
      const data = await response.json().catch(() => ({}));
      throw new Error((data as { error?: { description?: string } }).error?.description ?? "Queue action failed");
    }
  }

  async function bulkReview(decision: "approve" | "block") {
    setPending(true);
    setMessage("");
    try {
      for (const id of selected) {
        await run(`/v1/risk/events/${id}/review`, { decision, notes: `bulk ${decision} from dashboard` });
      }
      setSelected([]);
      setMessage(`Applied ${decision} to ${selected.length} risk event(s).`);
      router.refresh();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Bulk review failed");
    } finally {
      setPending(false);
    }
  }

  async function bulkResolve() {
    setPending(true);
    setMessage("");
    try {
      for (const id of selected) {
        await run(`/v1/risk/events/${id}/resolve`);
      }
      setSelected([]);
      setMessage(`Resolved ${selected.length} risk event(s).`);
      router.refresh();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Bulk resolve failed");
    } finally {
      setPending(false);
    }
  }

  async function bulkAssign() {
    setPending(true);
    setMessage("");
    try {
      for (const id of selected) {
        await run(`/v1/risk/events/${id}/assign`, { assigned_to: "dashboard-operator" });
      }
      setSelected([]);
      setMessage(`Assigned ${selected.length} risk event(s).`);
      router.refresh();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Bulk assign failed");
    } finally {
      setPending(false);
    }
  }

  function toggle(id: string) {
    setSelected((current) => (current.includes(id) ? current.filter((item) => item !== id) : [...current, id]));
  }

  function saveView() {
    const name = window.prompt("Saved view name");
    if (!name) return;
    setSavedViews((current) => [...current, { name, query, status, action, assigned }]);
  }

  function applyView(view: SavedView) {
    setQuery(view.query);
    setStatus(view.status);
    setAction(view.action);
    setAssigned(view.assigned);
  }

  return (
    <div className="stack">
      <div className="workbench-grid">
        <div className="detail-card">
          <div className="section-head">
            <div>
              <div className="eyebrow">Queue Filters</div>
              <h2>Workbench controls</h2>
              <div className="section-kicker">Shape the queue, preserve views, and narrow operator attention.</div>
            </div>
          </div>
          <div className="stack" style={{ marginTop: "16px" }}>
            <div className="control-form-grid">
              <label style={{ minWidth: "220px" }}>
                Search
                <input value={query} onChange={(event) => setQuery(event.target.value)} type="text" placeholder="payment, rule, assignee" />
              </label>
              <label>
                Resolution
                <select value={status} onChange={(event) => setStatus(event.target.value)}>
                  <option value="open">open</option>
                  <option value="resolved">resolved</option>
                  <option value="all">all</option>
                </select>
              </label>
              <label>
                Action
                <select value={action} onChange={(event) => setAction(event.target.value)}>
                  <option value="all">all</option>
                  <option value="allow">allow</option>
                  <option value="hold">hold</option>
                  <option value="block">block</option>
                </select>
              </label>
              <label>
                Assignment
                <select value={assigned} onChange={(event) => setAssigned(event.target.value)}>
                  <option value="all">all</option>
                  <option value="assigned">assigned</option>
                  <option value="unassigned">unassigned</option>
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
              <p>Risk events currently matching this workbench view.</p>
            </div>
            <div className="status-block">
              <span>Selected</span>
              <strong>{selected.length}</strong>
              <p>Events staged for bulk assignment, approval, block, or resolution.</p>
            </div>
            <div className="status-block">
              <span>Saved views</span>
              <strong>{savedViews.length}</strong>
              <p>Persistent query snapshots stored on this operator device.</p>
            </div>
          </div>
          <div className="hero-actions">
            <button className="ghost-button" type="button" disabled={pending || selected.length === 0} onClick={bulkAssign}>Assign</button>
            <button className="ghost-button" type="button" disabled={pending || selected.length === 0} onClick={() => bulkReview("approve")}>Approve</button>
            <button className="ghost-button" type="button" disabled={pending || selected.length === 0} onClick={() => bulkReview("block")}>Block</button>
            <button className="ghost-button" type="button" disabled={pending || selected.length === 0} onClick={bulkResolve}>Resolve</button>
          </div>
          {message ? <p className={`notice ${message.includes("failed") || message.includes("error") ? "error" : "success"}`}>{message}</p> : null}
        </div>
      </div>

      <div className="list-card">
        <div className="section-head">
          <div>
            <div className="eyebrow">Live Queue</div>
            <h2>Risk event ledger</h2>
          </div>
        </div>
        {visibleItems.length === 0 ? (
          <div className="empty-state">
            <strong>No risk events match this view</strong>
            <span className="muted">Broaden the filters or use a different saved view.</span>
          </div>
        ) : (
          visibleItems.map((ev) => (
            <article className="list-row" key={ev.id}>
              <div className="row-actions">
                <input type="checkbox" checked={selected.includes(ev.id)} onChange={() => toggle(ev.id)} aria-label={`Select ${ev.id}`} />
              </div>
              <div style={{ flex: 1 }}>
                <div className="row-title">{ev.payment_id}</div>
                <div className="row-meta">
                  <span className={actionBadge(ev.action)}>{ev.action}</span>
                  <span>score: {ev.score}</span>
                  {ev.review_status ? <span>{ev.review_status}</span> : null}
                  {ev.assigned_to ? <span>assigned to {ev.assigned_to}</span> : <span>unassigned</span>}
                  {ev.manual_decision ? <span>decision: {ev.manual_decision}</span> : null}
                  {ev.resolved ? <span className="badge-success">resolved</span> : <span className="badge-warning">open</span>}
                  <span>{formatTime(ev.created_at)}</span>
                </div>
                {ev.triggered_rules?.length > 0 ? <div className="muted">{ev.triggered_rules.join(", ")}</div> : null}
                {ev.review_notes ? <div className="muted">{ev.review_notes}</div> : null}
              </div>
              <div className="amount-pill">Score {ev.score}</div>
            </article>
          ))
        )}
      </div>
    </div>
  );
}
