import { getRefunds, requireViewer } from "../../lib/api";
import { type RefundItem, formatMoney, formatTime } from "../../lib/types";

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
      <section className="stack fade-up">
        <div className="ops-grid">
          <div className="hero-card">
            <div className="eyebrow">Refund Console</div>
            <h1>Refund review lane.</h1>
            <p className="lede">
              This surface is keyed to a payment. Open a payment record first, then enter its refund
              history from the payment detail page.
            </p>
          </div>

          <div className="detail-card">
            <div className="eyebrow">Routing Hint</div>
            <div className="status-matrix">
              <div className="status-block">
                <span>Entry path</span>
                <strong>Payment detail</strong>
                <p>Refund history is intentionally anchored to one payment lineage at a time.</p>
              </div>
              <div className="status-block">
                <span>Query input</span>
                <strong>payment_id</strong>
                <p>This page expects a payment identifier in the URL query string.</p>
              </div>
              <div className="status-block">
                <span>Current state</span>
                <strong>Idle</strong>
                <p>No payment has been selected for refund inspection yet.</p>
              </div>
            </div>
          </div>
        </div>
      </section>
    );
  }

  let data: { items: RefundItem[]; count: number };
  let fetchError: string | null = null;

  try {
    data = await getRefunds(paymentID);
  } catch (err) {
    fetchError = err instanceof Error ? err.message : "Failed to load refunds";
    data = { items: [], count: 0 };
  }

  return (
    <section className="stack fade-up">
      <div className="ops-grid">
        <div className="hero-card">
          <div className="eyebrow">Refund Console</div>
          <h1>Refunds for {paymentID}</h1>
          {!fetchError && (
            <p className="lede">
              {data.count} refund{data.count !== 1 ? "s" : ""} for this payment.
            </p>
          )}
        </div>

        <div className="detail-card">
          <div className="eyebrow">Refund Posture</div>
          <div className="status-matrix">
            <div className="status-block">
              <span>Refund count</span>
              <strong>{fetchError ? "—" : data.count}</strong>
              <p>Refund records currently linked to the selected payment lineage.</p>
            </div>
            <div className="status-block">
              <span>Payment anchor</span>
              <strong>{paymentID}</strong>
              <p>The exact payment ID this refund surface is scoped to.</p>
            </div>
            <div className="status-block">
              <span>Read state</span>
              <strong>{fetchError ? "Faulted" : "Loaded"}</strong>
              <p>Whether the refund history request succeeded for this payment context.</p>
            </div>
          </div>
        </div>
      </div>
      {fetchError ? (
        <div className="notice error">{fetchError}</div>
      ) : (
        <div className="list-card">
          <div className="section-head">
            <div>
              <div className="eyebrow">Refund Ledger</div>
              <h2>Issued refunds</h2>
            </div>
          </div>
          {data.items.length === 0 ? (
            <div className="empty-state">
              <strong>No refunds issued</strong>
              <span className="muted">This payment has not been partially or fully refunded.</span>
            </div>
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
