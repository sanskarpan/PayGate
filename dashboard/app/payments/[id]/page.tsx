import Link from "next/link";

import { getPayment, getUPIIntent } from "../../../lib/api";
import { formatMoney, formatTime, truncateMiddle } from "../../../lib/types";

export default async function PaymentDetailPage({ params }: { params: { id: string } }) {
  const payment = await getPayment(params.id);
  const upiIntent = payment.method === "upi" ? await getUPIIntent(params.id) : null;
  const timeline = [
    { label: "Created", at: payment.created_at, active: true },
    { label: "Customer Action", at: upiIntent ? payment.created_at : 0, active: payment.status === "pending_customer_action" || payment.method === "upi" },
    { label: "Processing", at: upiIntent?.last_polled_at || upiIntent?.completed_at || 0, active: payment.status === "processing" },
    { label: "Authorized", at: payment.authorized_at, active: Boolean(payment.authorized_at) },
    { label: "Captured", at: payment.captured_at, active: Boolean(payment.captured_at) },
  ];

  return (
    <section className="stack fade-up">
      <div className="hero-card">
        <div className="eyebrow">Payment Trace</div>
        <h1>{truncateMiddle(payment.id, 14, 8)}</h1>
        <p className="mono" style={{ margin: "0 0 8px" }}>{payment.id}</p>
        <p className="lede">
          {payment.status} · {formatMoney(payment.amount, payment.currency)} via {payment.method}.
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
      <div className="key-facts">
        <div className="key-fact">
          <span className="eyebrow">Status</span>
          <strong>{payment.status}</strong>
        </div>
        <div className="key-fact">
          <span className="eyebrow">Captured</span>
          <strong>{payment.captured ? "Yes" : "No"}</strong>
        </div>
        <div className="key-fact">
          <span className="eyebrow">Method</span>
          <strong>{payment.method}</strong>
        </div>
        {upiIntent ? (
          <div className="key-fact">
            <span className="eyebrow">UPI Status</span>
            <strong>{upiIntent.provider_status}</strong>
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
            {upiIntent ? (
              <>
                <div>
                  <dt>VPA</dt>
                  <dd>{upiIntent.vpa || "Not available"}</dd>
                </div>
                <div>
                  <dt>Provider Status</dt>
                  <dd>{upiIntent.provider_status}</dd>
                </div>
                <div>
                  <dt>Expires At</dt>
                  <dd>{formatTime(upiIntent.expires_at)}</dd>
                </div>
                <div>
                  <dt>Gateway Reference</dt>
                  <dd>{upiIntent.gateway_reference || "Pending"}</dd>
                </div>
                <div>
                  <dt>Intent Link</dt>
                  <dd>
                    {upiIntent.next_action?.deep_link ? (
                      <a href={upiIntent.next_action.deep_link} target="_blank" rel="noreferrer">
                        Open UPI App
                      </a>
                    ) : (
                      "Not available"
                    )}
                  </dd>
                </div>
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
