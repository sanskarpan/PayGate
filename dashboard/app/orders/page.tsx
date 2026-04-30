import Link from "next/link";

import { getOrders, requireViewer } from "../../lib/api";
import { formatMoney, formatTime } from "../../lib/types";

export default async function OrdersPage() {
  const viewer = await requireViewer();
  const orders = await getOrders();

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Merchant Scope</div>
        <h1>Orders</h1>
        <p className="lede">
          Reviewing {orders.count} order records for {viewer.merchant_id}.
        </p>
      </div>
      <div className="list-card">
        {orders.items.length === 0 ? (
          <p className="muted">No orders exist for this merchant yet.</p>
        ) : (
          orders.items.map((order) => (
            <Link className="list-row" href={`/orders/${order.id}`} key={order.id}>
              <div>
                <div className="row-title">{order.id}</div>
                <div className="row-meta">
                  <span>{order.status}</span>
                  <span>{order.receipt || "No receipt"}</span>
                  <span>{formatTime(order.created_at)}</span>
                </div>
              </div>
              <div className="amount-pill">{formatMoney(order.amount, order.currency)}</div>
            </Link>
          ))
        )}
      </div>
    </section>
  );
}
