"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

import type { DisputeItem } from "../lib/types";

export default function DisputeActionPanel({
  apiBaseUrl,
  dispute,
}: {
  apiBaseUrl: string;
  dispute: DisputeItem;
}) {
  const router = useRouter();
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [evidenceText, setEvidenceText] = useState(
    JSON.stringify(
      {
        customer_contacted: true,
        delivery_status: "fulfilled",
        note: "Attach merchant evidence here",
      },
      null,
      2,
    ),
  );
  const [resolveOutcome, setResolveOutcome] = useState("won");
  const [notes, setNotes] = useState("");

  async function run(path: string, body?: unknown) {
    setPending(true);
    setMessage("");
    try {
      const response = await fetch(`${apiBaseUrl}${path}`, {
        method: "POST",
        credentials: "include",
        headers: body ? { "Content-Type": "application/json" } : undefined,
        body: body ? JSON.stringify(body) : undefined,
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        setMessage((data as { error?: { description?: string } }).error?.description ?? "Action failed");
        return;
      }
      setMessage("Action completed.");
      router.refresh();
    } catch {
      setMessage("Network error while executing the dispute action.");
    } finally {
      setPending(false);
    }
  }

  async function submitEvidence() {
    try {
      const parsed = JSON.parse(evidenceText);
      await run(`/v1/disputes/${dispute.id}/submit-evidence`, parsed);
    } catch {
      setMessage("Evidence must be valid JSON.");
    }
  }

  if (["won", "lost", "accepted"].includes(dispute.status)) {
    return null;
  }

  return (
    <div className="detail-card">
      <div className="section-head">
        <div>
          <h2>Inline actions</h2>
          <div className="section-kicker">Execute dispute operations directly from the dashboard</div>
        </div>
      </div>

      <div className="stack" style={{ marginTop: "16px" }}>
        {dispute.status === "open" ? (
          <>
            <div className="spotlight-card">
              <div className="row-title">Submit evidence</div>
              <textarea
                onChange={(event) => setEvidenceText(event.target.value)}
                rows={10}
                style={{ width: "100%" }}
                value={evidenceText}
              />
              <div className="hero-actions">
                <button className="primary-button" disabled={pending} onClick={submitEvidence} type="button">
                  {pending ? "Submitting..." : "Submit Evidence"}
                </button>
              </div>
            </div>

            <div className="detail-grid">
              <div className="spotlight-card">
                <div className="row-title">Move to review</div>
                <div className="muted">Use once evidence is ready for adjudication.</div>
                <button
                  className="ghost-button"
                  disabled={pending}
                  onClick={() => run(`/v1/disputes/${dispute.id}/review`)}
                  type="button"
                >
                  Start Review
                </button>
              </div>
              <div className="spotlight-card">
                <div className="row-title">Accept dispute</div>
                <div className="muted">Concede the dispute without contesting the outcome.</div>
                <button
                  className="ghost-button"
                  disabled={pending}
                  onClick={() => run(`/v1/disputes/${dispute.id}/accept`)}
                  type="button"
                >
                  Accept Liability
                </button>
              </div>
            </div>
          </>
        ) : null}

        {dispute.status === "under_review" ? (
          <div className="spotlight-card">
            <div className="row-title">Resolve dispute</div>
            <div className="inline-form">
              <label>
                Outcome
                <select onChange={(event) => setResolveOutcome(event.target.value)} value={resolveOutcome}>
                  <option value="won">won</option>
                  <option value="lost">lost</option>
                  <option value="accepted">accepted</option>
                </select>
              </label>
              <label style={{ minWidth: "320px" }}>
                Notes
                <input onChange={(event) => setNotes(event.target.value)} type="text" value={notes} />
              </label>
            </div>
            <div className="hero-actions">
              <button
                className="primary-button"
                disabled={pending}
                onClick={() =>
                  run(`/v1/disputes/${dispute.id}/resolve`, {
                    outcome: resolveOutcome,
                    notes,
                  })
                }
                type="button"
              >
                {pending ? "Resolving..." : "Resolve Dispute"}
              </button>
            </div>
          </div>
        ) : null}

        {message ? (
          <p className={`notice ${message === "Action completed." ? "success" : "error"}`}>{message}</p>
        ) : null}
      </div>
    </div>
  );
}
