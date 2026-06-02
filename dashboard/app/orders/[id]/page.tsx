import Link from "next/link";

import { getOrder, getPayments } from "../../../lib/api";
import { formatMoney, formatTime, truncateMiddle } from "../../../lib/types";

export default async function OrderDetailPage({ params }: { params: { id: string } }) {
  const order = await getOrder(params.id);
  const payments = await getPayments(params.id);

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card">
          <div className="eyebrow">Order Dossier</div>
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

        <div className="detail-card">
          <div className="eyebrow">Order Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Commercial state</span>
              <strong>{order.status}</strong>
              <p>Current order lifecycle state as seen by downstream payment and settlement flows.</p>
            </div>
            <div className="status-block">
              <span>Collection gap</span>
              <strong>{formatMoney(order.amount_due, order.currency)}</strong>
              <p>Outstanding amount still not collected against the order principal.</p>
            </div>
            <div className="status-block">
              <span>Receipt anchor</span>
              <strong>{order.receipt || "Unset"}</strong>
              <p>Human-facing trace field used by merchant support and operator follow-through.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Total principal</span>
          <strong>{formatMoney(order.amount, order.currency)}</strong>
          <span>Original payable amount registered on the order.</span>
        </div>
        <div className="ops-band-item">
          <span>Collected</span>
          <strong>{formatMoney(order.amount_paid, order.currency)}</strong>
          <span>Amount successfully tied to completed or captured payment flow.</span>
        </div>
        <div className="ops-band-item">
          <span>Still due</span>
          <strong>{formatMoney(order.amount_due, order.currency)}</strong>
          <span>Residual amount not yet satisfied by available payment attempts.</span>
        </div>
        <div className="ops-band-item">
          <span>Attempt count</span>
          <strong>{payments.count}</strong>
          <span>Recorded payment traces currently linked to this order.</span>
        </div>
      </div>

      <div className="detail-grid">
        <div className="detail-card">
          <h2>Amount ledger</h2>
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
          <h2>Linked payment traces</h2>
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
