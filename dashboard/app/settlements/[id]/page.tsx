import { getSettlement } from "../../../lib/api";
import { formatMoney, formatTime } from "../../../lib/types";

export default async function SettlementDetailPage({ params }: { params: { id: string } }) {
  const settlement = await getSettlement(params.id);

  return (
    <section className="stack">
      <div className="hero-card">
        <div className="eyebrow">Settlement Batch</div>
        <h1>{settlement.id}</h1>
        <p className="lede">
          Net payout: {formatMoney(settlement.net_amount, settlement.currency)} ·{" "}
          {settlement.payment_count} payment{settlement.payment_count !== 1 ? "s" : ""}
        </p>
      </div>
      <div className="detail-grid">
        <div className="detail-card">
          <h2>Summary</h2>
          <dl className="detail-list">
            <div>
              <dt>Gross Amount</dt>
              <dd>{formatMoney(settlement.total_amount, settlement.currency)}</dd>
            </div>
            <div>
              <dt>Platform Fees</dt>
              <dd>− {formatMoney(settlement.total_fees, settlement.currency)}</dd>
            </div>
            <div>
              <dt>Refunds</dt>
              <dd>− {formatMoney(settlement.total_refunds, settlement.currency)}</dd>
            </div>
            <div>
              <dt>Net Payout</dt>
              <dd>
                <strong>{formatMoney(settlement.net_amount, settlement.currency)}</strong>
              </dd>
            </div>
            <div>
              <dt>Status</dt>
              <dd>{settlement.status}</dd>
            </div>
            <div>
              <dt>Period</dt>
              <dd>
                {formatTime(settlement.period_start)} → {formatTime(settlement.period_end)}
              </dd>
            </div>
            <div>
              <dt>Processed At</dt>
              <dd>{settlement.processed_at ? formatTime(settlement.processed_at) : "Pending"}</dd>
            </div>
          </dl>
        </div>
        <div className="detail-card">
          <h2>Payment Items ({settlement.items?.length ?? 0})</h2>
          <div className="list-card">
            {(settlement.items ?? []).map((item) => (
              <div className="list-row" key={item.id}>
                <div>
                  <div className="row-title">{item.payment_id}</div>
                  <div className="row-meta">
                    <span>Gross: {formatMoney(item.amount, item.currency)}</span>
                    <span>Fee: {formatMoney(item.fee, item.currency)}</span>
                    {item.refunds > 0 && (
                      <span>Refunds: {formatMoney(item.refunds, item.currency)}</span>
                    )}
                  </div>
                </div>
                <div className="amount-pill">{formatMoney(item.net, item.currency)}</div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
