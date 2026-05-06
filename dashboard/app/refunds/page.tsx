import { getRefunds, requireViewer } from "../../lib/api";
import { formatMoney, formatTime } from "../../lib/types";

function statusBadge(status: string) {
  if (status === "processed") return "badge-success";
  if (status === "processing") return "badge-info";
  if (status === "created") return "badge-warning";
  if (status === "failed") return "badge-error";
  return "badge-warning";
}

// The refund console is accessed via a query param: /refunds?payment_id=pay_xxx
// This page calls the API directly with the payment_id from the query string.
export default async function RefundsPage({
  searchParams,
}: {
  searchParams: { payment_id?: string };
}) {
  await requireViewer();
  const paymentID = searchParams.payment_id;

  if (!paymentID) {
    return (
      <section className="stack">
        <div className="hero-card">
          <div className="eyebrow">Refund Console</div>
          <h1>Refunds</h1>
          <p className="lede">
            Navigate to a payment and click &ldquo;View Refunds&rdquo; to see refund history.
          </p>
        </div>
      </section>
    );
  }

  let data: { items: Array<{ id: string; amount: number; currency: string; status: string; reason: string; created_at: number }>; count: number };
  let fetchError: string | null = null;

  try {
    data = await getRefunds(paymentID);
  } catch (err) {
    fetchError = err instanceof Error ? err.message : "Failed to load refunds";
    data = { items: [], count: 0 };
  }

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Refund Console</div>
        <h1>Refunds for {paymentID}</h1>
        {!fetchError && (
          <p className="lede">
            {data.count} refund{data.count !== 1 ? "s" : ""} for this payment.
          </p>
        )}
      </div>
      {fetchError ? (
        <div className="notice error">{fetchError}</div>
      ) : (
        <div className="list-card">
          {data.items.length === 0 ? (
            <p className="muted">No refunds have been issued for this payment.</p>
          ) : (
            data.items.map((ref) => (
              <div className="list-row" key={ref.id}>
                <div>
                  <div className="row-title">{ref.id}</div>
                  <div className="row-meta">
                    <span className={statusBadge(ref.status)}>{ref.status}</span>
                    {ref.reason && <span>{ref.reason}</span>}
                    <span>{formatTime(ref.created_at)}</span>
                  </div>
                </div>
                <div className="amount-pill">{formatMoney(ref.amount, ref.currency)}</div>
              </div>
            ))
          )}
        </div>
      )}
    </section>
  );
}
