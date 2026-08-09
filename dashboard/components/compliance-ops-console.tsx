"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

import type {
  CapabilityItem,
  OnboardingApplicationItem,
} from "../lib/types";
import { RouteIcon } from "./system-icons";

type Props = {
  merchantID: string;
  onboarding: OnboardingApplicationItem;
  capabilities: CapabilityItem[];
};

type CapabilityStatus = "enabled" | "restricted" | "disabled";
type OnboardingDecision = "in_review" | "needs_information" | "approved" | "rejected";

export default function ComplianceOpsConsole({ merchantID, onboarding, capabilities }: Props) {
  const router = useRouter();
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [reviewState, setReviewState] = useState<OnboardingDecision>("in_review");
  const [reviewNotes, setReviewNotes] = useState("");

  async function run(path: string, body: unknown, success: string, method: "POST" | "PUT" = "POST") {
    setPending(true);
    setMessage("");
    try {
      const response = await fetch(`/api/proxy${path}`, {
        method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        setMessage((data as { error?: { description?: string } }).error?.description ?? "Action failed");
        return false;
      }
      setMessage(success);
      router.refresh();
      return true;
    } catch {
      setMessage("Network error while updating compliance state.");
      return false;
    } finally {
      setPending(false);
    }
  }

  async function reviewOnboarding() {
    await run(
      "/v1/merchants/me/onboarding/review",
      {
        merchant_id: merchantID,
        state: reviewState,
        reviewer_notes: reviewNotes,
      },
      "Onboarding review updated.",
    );
  }

  async function setCapability(item: CapabilityItem, nextStatus: CapabilityStatus) {
    const reason =
      nextStatus === "enabled"
        ? "operator enabled capability from dashboard"
        : "operator restricted capability from dashboard";
    await run(
      "/v1/merchants/me/capabilities",
      {
        items: [
          {
            capability_code: item.capability_code,
            status: nextStatus,
            reason,
          },
        ],
      },
      `Capability ${item.capability_code} updated.`,
      // The capabilities route is registered as PUT; posting to it returns 405.
      "PUT",
    );
  }

  return (
    <div className="workbench-grid">
      <div className="detail-card">
        <div className="section-head">
          <div>
            <div className="eyebrow">Review Rail</div>
            <h2>Onboarding decision</h2>
            <div className="section-kicker">Move the merchant into review, request more information, approve, or reject.</div>
          </div>
        </div>
        <div className="page-glyph" style={{ position: "absolute", right: "18px", top: "18px" }}>
          <div className="page-glyph-badge">
            <RouteIcon name="compliance" size={36} />
          </div>
          <div className="page-glyph-label">
            <span>Review file</span>
            <strong>Decision rail</strong>
          </div>
        </div>
        <div className="stack" style={{ marginTop: "16px" }}>
          <div className="control-form-grid">
            <label>
              Decision
              <select value={reviewState} onChange={(event) => setReviewState(event.target.value as OnboardingDecision)}>
                <option value="in_review">in_review</option>
                <option value="needs_information">needs_information</option>
                <option value="approved">approved</option>
                <option value="rejected">rejected</option>
              </select>
            </label>
            <label style={{ minWidth: "320px" }}>
              Reviewer notes
              <input
                type="text"
                value={reviewNotes}
                onChange={(event) => setReviewNotes(event.target.value)}
                placeholder="Explain the operator decision"
              />
            </label>
          </div>
          <div className="hero-actions">
            <button className="primary-button" type="button" disabled={pending} onClick={reviewOnboarding}>
              {pending ? "Updating..." : "Apply review decision"}
            </button>
          </div>
        </div>
      </div>

      <div className="detail-card">
        <div className="eyebrow">Capability Posture</div>
        <div className="status-matrix">
          <div className="status-block">
            <span>Application state</span>
            <strong>{onboarding.state}</strong>
            <p>Latest submitted onboarding state at the time this console was rendered.</p>
          </div>
          <div className="status-block">
            <span>Capabilities</span>
            <strong>{capabilities.length}</strong>
            <p>Total live merchant capability rails visible to this operator.</p>
          </div>
          <div className="status-block">
            <span>Enabled rails</span>
            <strong>{capabilities.filter((item) => item.status === "enabled").length}</strong>
            <p>Capabilities currently admitted for production use.</p>
          </div>
        </div>
      </div>

      <div className="detail-card" style={{ gridColumn: "1 / -1" }}>
        <div className="section-head with-glyph">
          <div>
            <div className="eyebrow">Capability Controls</div>
            <h2>Merchant live rails</h2>
            <div className="section-kicker">Enable or restrict merchant live capabilities from the same review surface.</div>
          </div>
        </div>
        <div className="stack" style={{ marginTop: "16px" }}>
          {capabilities.map((item) => (
            <div className="list-row" key={item.id}>
              <div>
                <div className="row-title">{item.capability_code}</div>
                <div className="row-meta">
                  <span className={item.status === "enabled" ? "badge-success" : "badge-warning"}>{item.status}</span>
                  {item.reason ? <span>{item.reason}</span> : null}
                </div>
              </div>
              <div className="row-actions">
                <button
                  className="ghost-button"
                  type="button"
                  disabled={pending || item.status === "enabled"}
                  onClick={() => setCapability(item, "enabled")}
                >
                  Enable
                </button>
                <button
                  className="ghost-button"
                  type="button"
                  disabled={pending || item.status === "restricted"}
                  onClick={() => setCapability(item, "restricted")}
                >
                  Restrict
                </button>
              </div>
            </div>
          ))}
        </div>
        {message ? <p className={`notice ${message.includes("failed") || message.includes("error") ? "error" : "success"}`}>{message}</p> : null}
      </div>
    </div>
  );
}
