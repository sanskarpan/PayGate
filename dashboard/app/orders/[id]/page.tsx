import Link from "next/link";

import { getOrder, getPayments } from "../../../lib/api";
import { formatMoney, formatTime } from "../../../lib/types";

export default async function OrderDetailPage({ params }: { params: { id: string } }) {
  const order = await getOrder(params.id);
  const payments = await getPayments(params.id);

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Order Detail</div>
        <h1>Order Detail</h1>
        <p className="muted" style={{ fontFamily: "monospace", fontSize: "0.9rem", margin: "4px 0 8px" }}>{order.id}</p>
        <p className="lede">
          {order.status} · {formatMoney(order.amount, order.currency)} · Receipt {order.receipt || "not set"}.
        </p>
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
            <p className="muted">No payment attempts recorded for this order.</p>
          ) : (
            payments.items.map((payment) => (
              <Link className="list-row" href={`/payments/${payment.id}`} key={payment.id}>
                <div>
                  <div className="row-title">{payment.id}</div>
                  <div className="row-meta">
                    <span>{payment.status}</span>
                    <span>{payment.method}</span>
                    <span>{formatTime(payment.created_at)}</span>
                  </div>
                </div>
                <div className="amount-pill">{formatMoney(payment.amount, payment.currency)}</div>
              </Link>
            ))
          )}
        </div>
      </div>
    </section>
  );
}
