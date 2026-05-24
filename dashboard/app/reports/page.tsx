import { getReportCatalog, getReportExports, getTaxProfile, requireViewer } from "../../lib/api";
import { formatMoney, formatTime } from "../../lib/types";

export default async function ReportsPage() {
  await requireViewer();
  const [catalog, exports, taxProfile] = await Promise.all([
    getReportCatalog(),
    getReportExports(),
    getTaxProfile(),
  ]);

  const periodEnd = Math.floor(Date.now() / 1000);
  const periodStart = periodEnd - 30 * 24 * 60 * 60;

  return (
    <section className="stack fade-up">
      <div className="hero-card">
        <div className="eyebrow">Reporting</div>
        <h1>Download center.</h1>
        <p className="lede">Generate finance exports, inspect completed files, and maintain tax profile defaults.</p>
        <div className="metric-strip">
          <div className="metric-chip">
            <span className="metric-chip-label">Report types</span>
            <strong>{catalog.count}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Exports</span>
            <strong>{exports.count}</strong>
          </div>
          <div className="metric-chip">
            <span className="metric-chip-label">Default tax rate</span>
            <strong>{taxProfile.default_tax_rate_bps / 100}%</strong>
          </div>
        </div>
      </div>

      <div className="detail-grid">
        <div className="glass-card">
          <div className="section-head">
            <div>
              <h2>Request export</h2>
              <div className="section-kicker">Default window uses the last 30 days</div>
            </div>
          </div>
          <form action="/api/proxy/v1/reports/exports" className="stack" method="POST" style={{ marginTop: "16px" }}>
            <input type="hidden" name="_redirect" value="/reports" />
            <input type="hidden" name="period_start" value={periodStart} />
            <input type="hidden" name="period_end" value={periodEnd} />
            <label>
              Report type
              <select name="report_type" defaultValue={catalog.items[0]?.report_type || "payments"}>
                {catalog.items.map((item) => (
                  <option key={item.report_type} value={item.report_type}>{item.label}</option>
                ))}
              </select>
            </label>
            <label>
              Format
              <select name="format" defaultValue="csv">
                <option value="csv">csv</option>
              </select>
            </label>
            <button className="primary-button" type="submit">Generate export</button>
          </form>
        </div>

        <div className="glass-card">
          <div className="section-head">
            <div>
              <h2>Tax profile</h2>
              <div className="section-kicker">Default values used in tax summaries and statements</div>
            </div>
          </div>
          <form action="/api/proxy/v1/reports/tax-profile" className="stack" method="POST" style={{ marginTop: "16px" }}>
            <input type="hidden" name="_redirect" value="/reports" />
            <input type="hidden" name="_method" value="PUT" />
            <label>
              Legal name
              <input defaultValue={taxProfile.legal_name} name="legal_name" type="text" />
            </label>
            <label>
              GSTIN
              <input defaultValue={taxProfile.gstin} name="gstin" type="text" />
            </label>
            <div className="inline-form">
              <label>
                State code
                <input defaultValue={taxProfile.business_state_code} name="business_state_code" type="text" />
              </label>
              <label>
                Place of supply
                <input defaultValue={taxProfile.place_of_supply} name="place_of_supply" type="text" />
              </label>
              <label>
                Tax rate (bps)
                <input defaultValue={taxProfile.default_tax_rate_bps} name="default_tax_rate_bps" type="number" />
              </label>
            </div>
            <button className="ghost-button" type="submit">Save tax profile</button>
          </form>
        </div>
      </div>

      <div className="list-card">
        <div className="section-head">
          <div>
            <h2>Available reports</h2>
            <div className="section-kicker">Catalog of statements and exportable finance surfaces</div>
          </div>
        </div>
        {catalog.items.map((item) => (
          <div className="list-row" key={item.report_type}>
            <div>
              <div className="row-title">{item.label}</div>
              <div className="row-meta">
                <span className="badge-info">{item.report_type}</span>
                <span>{item.supports_api ? "api available" : "manual only"}</span>
              </div>
              <div className="muted">{item.description}</div>
            </div>
          </div>
        ))}
      </div>

      <div className="list-card">
        <div className="section-head">
          <div>
            <h2>Export jobs</h2>
            <div className="section-kicker">Generated downloads, status, and signed download links</div>
          </div>
        </div>
        {exports.items.length === 0 ? (
          <div className="empty-state">
            <strong>No exports created yet</strong>
            <span className="muted">Generate the first report export to seed the download center.</span>
          </div>
        ) : (
          exports.items.map((item) => (
            <article className="list-row" key={item.id}>
              <div className="entity-primary">
                <div className="row-title">{item.file_name || item.report_type}</div>
                <div className="row-meta">
                  <span className="badge-neutral">{item.report_type}</span>
                  <span className={item.status === "completed" ? "badge-success" : item.status === "failed" ? "badge-error" : "badge-warning"}>
                    {item.status}
                  </span>
                  <span>{item.file_size_bytes ? formatMoney(item.file_size_bytes, "INR").replace("₹", "") + " bytes" : "preparing file"}</span>
                  <span>{formatTime(item.created_at)}</span>
                </div>
                {item.error_message ? <div className="muted">{item.error_message}</div> : null}
              </div>
              <div className="row-actions">
                {item.download_url ? (
                  <a className="ghost-button" href={item.download_url}>Download</a>
                ) : null}
              </div>
            </article>
          ))
        )}
      </div>
    </section>
  );
}
