import Link from "next/link";

import { getOrder, getPayments } from "../../../lib/api";
import { formatMoney, formatTime, truncateMiddle } from "../../../lib/types";

export default async function OrderDetailPage({ params }: { params: { id: string } }) {
  const order = await getOrder(params.id);
  const payments = await getPayments(params.id);

  return (
    <section className="stack fade-up">
      <div className="hero-card">
        <div className="eyebrow">Order Detail</div>
        <h1>{truncateMiddle(order.id, 14, 8)}</h1>
        <p className="mono" style={{ margin: "4px 0 8px" }}>{order.id}</p>
        <p className="lede">
          {order.status} order for {formatMoney(order.amount, order.currency)}.
          Receipt {order.receipt || "not set"}.
        </p>
        <div className="metric-strip">
          <div className="metric-chip">
            <span className="metric-chip-label">Amount due</span>
            <strong>{formatMoney(order.amount_due, order.currency)}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Amount paid</span>
            <strong>{formatMoney(order.amount_paid, order.currency)}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Payment attempts</span>
            <strong>{payments.count}</strong>
          </div>
        </div>
      </div>
      <div className="detail-grid">
        <div className="detail-card">
          <h2>Amounts</h2>
          <dl className="detail-list">
            <div>
              <dt>Total</dt>
              <dd>{formatMoney(order.amount, order.currency)}</dd>
            </div>
            <div>
              <dt>Paid</dt>
              <dd>{formatMoney(order.amount_paid, order.currency)}</dd>
            </div>
            <div>
              <dt>Due</dt>
              <dd>{formatMoney(order.amount_due, order.currency)}</dd>
            </div>
            <div>
              <dt>Created</dt>
              <dd>{formatTime(order.created_at)}</dd>
            </div>
          </dl>
        </div>
        <div className="detail-card">
          <h2>Linked Payments</h2>
          {payments.items.length === 0 ? (
            <div className="empty-state">
              <strong>No payment attempts</strong>
              <span className="muted">This order exists, but no payment authorization attempts were recorded.</span>
            </div>
          ) : (
            <div className="stack">
              {payments.items.map((payment) => (
                <Link className="list-row" href={`/payments/${payment.id}`} key={payment.id}>
                  <div className="entity-primary">
                    <div className="row-title">{truncateMiddle(payment.id)}</div>
                    <div className="row-meta">
                      <span className="badge-neutral">{payment.status}</span>
                      <span>{payment.method}</span>
                      <span>{payment.captured ? "Captured" : "Awaiting capture"}</span>
                      <span>{formatTime(payment.created_at)}</span>
                    </div>
                  </div>
                  <div className="amount-pill">{formatMoney(payment.amount, payment.currency)}</div>
                </Link>
              ))}
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
