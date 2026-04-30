import Link from "next/link";

import { getPayment } from "../../../lib/api";
import { formatMoney, formatTime } from "../../../lib/types";

export default async function PaymentDetailPage({ params }: { params: { id: string } }) {
  const payment = await getPayment(params.id);
  const timeline = [
    { label: "Created", at: payment.created_at, active: true },
    { label: "Authorized", at: payment.authorized_at, active: Boolean(payment.authorized_at) },
    { label: "Captured", at: payment.captured_at, active: Boolean(payment.captured_at) },
  ];

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Payment Trace</div>
        <h1>{payment.id}</h1>
        <p className="lede">
          {payment.status} · {formatMoney(payment.amount, payment.currency)} via {payment.method}.
        </p>
        <Link className="ghost-button" href={`/orders/${payment.order_id}`}>
          View Parent Order
        </Link>
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
              <dd>{payment.order_id}</dd>
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
          </dl>
        </div>
      </div>
    </section>
  );
}
