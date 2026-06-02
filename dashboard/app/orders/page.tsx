import Link from "next/link";

import { getOrders, requireViewer } from "../../lib/api";
import { formatCompactNumber, formatMoney, formatTime, truncateMiddle } from "../../lib/types";

export default async function OrdersPage({
  searchParams,
}: {
  searchParams: { cursor?: string };
}) {
  const viewer = await requireViewer();
  const orders = await getOrders(searchParams.cursor);
  const partialEnabled = orders.items.filter((item) => item.partial_payment).length;

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card">
          <div className="eyebrow">Money Intake</div>
          <h1>Orders</h1>
          <p className="lede">
            Reviewing {orders.count} order records for {viewer.merchant_id}. Use this view to
            inspect checkout creation patterns and drill directly into payment attempts.
          </p>
          <div className="metric-strip">
            <div className="metric-chip">
              <span className="metric-chip-label">Visible orders</span>
              <strong>{formatCompactNumber(orders.count)}</strong>
            </div>
            <div className="metric-chip">
              <span className="metric-chip-label">Pagination</span>
              <strong>{orders.has_more ? "More available" : "Complete slice"}</strong>
            </div>
          </div>
        </div>

        <div className="detail-card">
          <div className="eyebrow">Feed Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Order slice</span>
              <strong>{orders.count}</strong>
              <p>Current visible order records across the operator pagination window.</p>
            </div>
            <div className="status-block">
              <span>Partial payment</span>
              <strong>{partialEnabled}</strong>
              <p>Orders currently allowing split or staged commercial collection.</p>
            </div>
            <div className="status-block">
              <span>Feed depth</span>
              <strong>{orders.has_more ? "Extended" : "Terminal"}</strong>
              <p>Whether additional checkout intake records remain beyond the current slice.</p>
            </div>
          </div>
        </div>
      </div>

      <div className="ops-band">
        <div className="ops-band-item">
          <span>Visible orders</span>
          <strong>{orders.count}</strong>
          <span>Order records inside the current working set.</span>
        </div>
        <div className="ops-band-item">
          <span>Partial enabled</span>
          <strong>{partialEnabled}</strong>
          <span>Orders that can resolve through more than one collection event.</span>
        </div>
        <div className="ops-band-item">
          <span>Feed state</span>
          <strong>{orders.has_more ? "Expandable" : "Closed"}</strong>
          <span>Whether further intake records exist beyond this page.</span>
        </div>
        <div className="ops-band-item">
          <span>Merchant scope</span>
          <strong>{truncateMiddle(viewer.merchant_id)}</strong>
          <span>Current order surface tenant boundary.</span>
        </div>
      </div>

      <div className="list-card">
        <div className="section-head">
          <div>
            <h2>Order feed</h2>
            <div className="section-kicker">Status, receipt context, timestamp, and value</div>
          </div>
        </div>
        {orders.items.length === 0 ? (
          <div className="empty-state">
            <strong>No orders yet</strong>
            <span className="muted">New merchant checkout sessions will appear here once created.</span>
          </div>
        ) : (
          orders.items.map((order) => (
            <Link className="list-row" href={`/orders/${order.id}`} key={order.id}>
              <div className="entity-primary">
                <div className="row-title">{truncateMiddle(order.id)}</div>
                <div className="mono">{order.id}</div>
                <div className="row-meta">
                  <span className="badge-neutral">{order.status}</span>
                  <span>{order.receipt || "Receipt pending"}</span>
                  <span>{order.partial_payment ? "Partial payment enabled" : "Single-shot payment"}</span>
                  <span>{formatTime(order.created_at)}</span>
                </div>
              </div>
              <div className="amount-pill">{formatMoney(order.amount, order.currency)}</div>
            </Link>
          ))
        )}
      </div>
      {orders.has_more && orders.next_cursor && (
        <div style={{ textAlign: "center" }}>
          <Link
            className="ghost-button"
            href={`/orders?cursor=${encodeURIComponent(orders.next_cursor)}`}
          >
            Load more
          </Link>
        </div>
      )}
    </section>
  );
}
