import Link from "next/link";

import { getPayment, getUPIIntent } from "../../../lib/api";
import { formatMoney, formatTime, truncateMiddle } from "../../../lib/types";

export default async function PaymentDetailPage({ params }: { params: { id: string } }) {
  const payment = await getPayment(params.id);
  const upiIntent = payment.method === "upi" ? await getUPIIntent(params.id) : null;
  const isUPIQR = upiIntent?.flow_type === "qr";
  const isCardChallenge = payment.method === "card" && payment.status === "requires_action";
  const timeline = [
    { label: "Created", at: payment.created_at, active: true },
    {
      label: "Customer Action",
      at: isCardChallenge
        ? payment.card_challenge?.expires_at || payment.created_at
        : upiIntent
          ? payment.created_at
          : 0,
      active: payment.status === "pending_customer_action" || payment.method === "upi" || isCardChallenge,
    },
    { label: "Processing", at: upiIntent?.last_polled_at || upiIntent?.completed_at || 0, active: payment.status === "processing" },
    { label: "Authorized", at: payment.authorized_at, active: Boolean(payment.authorized_at) },
    { label: "Captured", at: payment.captured_at, active: Boolean(payment.captured_at) },
  ];

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card">
          <div className="eyebrow">Payment Trace</div>
          <h1>{truncateMiddle(payment.id, 14, 8)}</h1>
          <p className="mono" style={{ margin: "0 0 8px" }}>{payment.id}</p>
          <p className="lede">
            {payment.status} · {formatMoney(payment.amount, payment.currency)} via {payment.method}{isUPIQR ? " QR" : isCardChallenge ? " with 3DS" : ""}.
          </p>
          <div className="hero-actions">
            <Link className="ghost-button" href={`/orders/${payment.order_id}`}>
              View Parent Order
            </Link>
            <Link className="ghost-button" href={`/refunds?payment_id=${payment.id}`}>
              View Refunds
            </Link>
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Payment Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Lifecycle state</span>
              <strong>{payment.status}</strong>
              <p>Current payment state across creation, customer action, authorization, and capture.</p>
            </div>
            <div className="status-block">
              <span>Method lane</span>
              <strong>{payment.method}{isUPIQR ? " / QR" : isCardChallenge ? " / 3DS" : ""}</strong>
              <p>Execution lane driving the next-action surface and downstream operator handling.</p>
            </div>
            <div className="status-block">
              <span>Capture posture</span>
              <strong>{payment.captured ? "Captured" : "Pending"}</strong>
              <p>Whether commercial settlement can proceed without further capture intervention.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Status</span>
          <strong>{payment.status}</strong>
          <span>Current payment lifecycle state.</span>
        </div>
        <div className="ops-band-item">
          <span>Captured</span>
          <strong>{payment.captured ? "Yes" : "No"}</strong>
          <span>Whether the payment is fully captured for settlement.</span>
        </div>
        <div className="ops-band-item">
          <span>Method</span>
          <strong>{payment.method}</strong>
          <span>Primary commercial execution method on this trace.</span>
        </div>
        {isCardChallenge ? (
          <div className="ops-band-item">
            <span>Challenge</span>
            <strong>{payment.card_challenge?.status || payment.next_action?.challenge_status || "pending"}</strong>
            <span>3DS customer action posture for the current card flow.</span>
          </div>
        ) : upiIntent ? (
          <div className="ops-band-item">
            <span>UPI Status</span>
            <strong>{upiIntent.provider_status}</strong>
            <span>Provider-side status observed on the UPI interaction surface.</span>
          </div>
        ) : null}
      </div>
      <div className="detail-grid">
        <div className="detail-card">
          <h2>State History</h2>
          <div className="timeline">
            {timeline.map((entry) => (
              <div className={`timeline-item${entry.active ? " active" : ""}`} key={entry.label}>
                <div className="timeline-dot" />
                <div>
                  <div className="row-title">{entry.label}</div>
                  <div className="muted">{entry.active ? formatTime(entry.at) : "Pending"}</div>
                </div>
              </div>
            ))}
          </div>
        </div>
        <div className="detail-card">
          <h2>Attributes</h2>
          <dl className="detail-list">
            <div>
              <dt>Order ID</dt>
              <dd>{truncateMiddle(payment.order_id)}</dd>
            </div>
            <div>
              <dt>Captured</dt>
              <dd>{payment.captured ? "Yes" : "No"}</dd>
            </div>
            <div>
              <dt>Authorized</dt>
              <dd>{payment.authorized_at ? formatTime(payment.authorized_at) : "Not available"}</dd>
            </div>
            <div>
              <dt>Captured At</dt>
              <dd>{payment.captured_at ? formatTime(payment.captured_at) : "Not available"}</dd>
            </div>
            {isCardChallenge ? (
              <>
                <div>
                  <dt>Challenge Status</dt>
                  <dd>{payment.card_challenge?.status || payment.next_action?.challenge_status || "pending"}</dd>
                </div>
                <div>
                  <dt>Challenge Session</dt>
                  <dd>{payment.card_challenge?.session_id || payment.next_action?.challenge_session_id || "Not available"}</dd>
                </div>
                <div>
                  <dt>Redirect URL</dt>
                  <dd>
                    {payment.next_action?.redirect_url ? (
                      <a href={payment.next_action.redirect_url} target="_blank" rel="noreferrer">
                        Open 3DS Challenge
                      </a>
                    ) : (
                      "Not available"
                    )}
                  </dd>
                </div>
                <div>
                  <dt>Challenge Expires</dt>
                  <dd>{payment.next_action?.expires_at ? formatTime(payment.next_action.expires_at) : "Not available"}</dd>
                </div>
                <div>
                  <dt>Sandbox Callback</dt>
                  <dd>{payment.sandbox?.callback_url || payment.card_challenge?.sandbox_callback_url || "Not available"}</dd>
                </div>
              </>
            ) : null}
            {upiIntent ? (
              <>
                <div>
                  <dt>{isUPIQR ? "Display Name" : "VPA"}</dt>
                  <dd>{isUPIQR ? upiIntent.display_name || "PayGate" : upiIntent.vpa || "Not available"}</dd>
                </div>
                <div>
                  <dt>Provider Status</dt>
                  <dd>{upiIntent.provider_status}</dd>
                </div>
                {isUPIQR ? (
                  <div>
                    <dt>QR Mode</dt>
                    <dd>{upiIntent.qr_mode || "dynamic"}</dd>
                  </div>
                ) : null}
                <div>
                  <dt>Expires At</dt>
                  <dd>{formatTime(upiIntent.expires_at)}</dd>
                </div>
                <div>
                  <dt>Gateway Reference</dt>
                  <dd>{upiIntent.gateway_reference || "Pending"}</dd>
                </div>
                <div>
                  <dt>{isUPIQR ? "Scan Payload" : "Intent Link"}</dt>
                  <dd>
                    {isUPIQR
                      ? upiIntent.next_action?.qr_payload || upiIntent.qr_payload || "Not available"
                      : upiIntent.next_action?.deep_link ? (
                          <a href={upiIntent.next_action.deep_link} target="_blank" rel="noreferrer">
                            Open UPI App
                          </a>
                        ) : (
                          "Not available"
                        )}
                  </dd>
                </div>
                {isUPIQR ? (
                  <div>
                    <dt>Reusable</dt>
                    <dd>{upiIntent.is_reusable ? "Yes" : "No"}</dd>
                  </div>
                ) : null}
                <div>
                  <dt>Failure</dt>
                  <dd>{upiIntent.failure_code || upiIntent.failure_description || "Not available"}</dd>
                </div>
              </>
            ) : null}
          </dl>
        </div>
      </div>
    </section>
  );
}
