package reporting

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/common/protect"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) UpsertTaxProfile(ctx context.Context, in TaxProfile) (TaxProfile, error) {
	if in.DefaultTaxRateBPS <= 0 {
		in.DefaultTaxRateBPS = 1800
	}
	encryptedGSTIN, err := protect.Default().SealStringForDomain(protect.DomainReportingTaxProfile, in.GSTIN)
	if err != nil {
		return TaxProfile{}, fmt.Errorf("encrypt gstin: %w", err)
	}
	_, err = r.db.Exec(ctx, `
INSERT INTO paygate_reporting.tax_profiles
    (merchant_id, legal_name, gstin, business_state_code, place_of_supply, default_tax_rate_bps)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (merchant_id) DO UPDATE
SET legal_name = EXCLUDED.legal_name,
    gstin = EXCLUDED.gstin,
    business_state_code = EXCLUDED.business_state_code,
    place_of_supply = EXCLUDED.place_of_supply,
    default_tax_rate_bps = EXCLUDED.default_tax_rate_bps,
    updated_at = NOW()
`, in.MerchantID, in.LegalName, encryptedGSTIN, in.BusinessStateCode, in.PlaceOfSupply, in.DefaultTaxRateBPS)
	if err != nil {
		return TaxProfile{}, fmt.Errorf("upsert tax profile: %w", err)
	}
	return r.GetTaxProfile(ctx, in.MerchantID)
}

func (r *Repository) GetTaxProfile(ctx context.Context, merchantID string) (TaxProfile, error) {
	var out TaxProfile
	err := r.db.QueryRow(ctx, `
SELECT merchant_id, legal_name, gstin, business_state_code, place_of_supply, default_tax_rate_bps, created_at, updated_at
FROM paygate_reporting.tax_profiles
WHERE merchant_id = $1
`, merchantID).Scan(&out.MerchantID, &out.LegalName, &out.GSTIN, &out.BusinessStateCode, &out.PlaceOfSupply, &out.DefaultTaxRateBPS, &out.CreatedAt, &out.UpdatedAt)
	if err == nil {
		out.GSTIN, err = protect.Default().OpenStringForDomain(protect.DomainReportingTaxProfile, out.GSTIN)
		if err != nil {
			return TaxProfile{}, fmt.Errorf("decrypt gstin: %w", err)
		}
		return out, nil
	}
	if err != pgx.ErrNoRows {
		return TaxProfile{}, fmt.Errorf("get tax profile: %w", err)
	}
	now := time.Now().UTC()
	return TaxProfile{
		MerchantID:        merchantID,
		DefaultTaxRateBPS: 1800,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

func (r *Repository) BuildStatement(ctx context.Context, merchantID string, reportType ReportType, start, end time.Time) (Statement, error) {
	profile, err := r.GetTaxProfile(ctx, merchantID)
	if err != nil {
		return Statement{}, err
	}
	stmt := Statement{
		EntityType:  string(reportType),
		PeriodStart: start,
		PeriodEnd:   end,
		TaxRateBPS:  profile.DefaultTaxRateBPS,
		Totals:      map[string]int64{},
		Rows:        []map[string]any{},
	}
	switch reportType {
	case ReportTypePayments:
		rows, err := r.db.Query(ctx, `
SELECT id, order_id, amount, currency, method, status, fee, COALESCE(method_state, ''), created_at, COALESCE(captured_at, created_at)
FROM paygate_payments.payments
WHERE merchant_id = $1 AND created_at >= $2 AND created_at < $3
ORDER BY created_at DESC
`, merchantID, start, end)
		if err != nil {
			return Statement{}, err
		}
		defer rows.Close()
		var gross, fees int64
		for rows.Next() {
			var id, orderID, currency, method, status, methodState string
			var amount, fee int64
			var createdAt, activityAt time.Time
			if err := rows.Scan(&id, &orderID, &amount, &currency, &method, &status, &fee, &methodState, &createdAt, &activityAt); err != nil {
				return Statement{}, err
			}
			gross += amount
			fees += fee
			stmt.Rows = append(stmt.Rows, map[string]any{
				"id":           id,
				"order_id":     orderID,
				"amount":       amount,
				"currency":     currency,
				"method":       method,
				"status":       status,
				"method_state": methodState,
				"fee":          fee,
				"created_at":   createdAt,
				"activity_at":  activityAt,
			})
		}
		stmt.Totals["count"] = int64(len(stmt.Rows))
		stmt.Totals["gross_amount"] = gross
		stmt.Totals["fee_amount"] = fees
		stmt.Totals["tax_amount"] = calculateTax(fees, profile.DefaultTaxRateBPS)
	case ReportTypeRefunds:
		rows, err := r.db.Query(ctx, `
SELECT id, payment_id, amount, currency, reason, status, created_at, COALESCE(processed_at, created_at)
FROM paygate_refunds.refunds
WHERE merchant_id = $1 AND created_at >= $2 AND created_at < $3
ORDER BY created_at DESC
`, merchantID, start, end)
		if err != nil {
			return Statement{}, err
		}
		defer rows.Close()
		var total int64
		for rows.Next() {
			var id, paymentID, currency, reason, status string
			var amount int64
			var createdAt, activityAt time.Time
			if err := rows.Scan(&id, &paymentID, &amount, &currency, &reason, &status, &createdAt, &activityAt); err != nil {
				return Statement{}, err
			}
			total += amount
			stmt.Rows = append(stmt.Rows, map[string]any{
				"id":          id,
				"payment_id":  paymentID,
				"amount":      amount,
				"currency":    currency,
				"reason":      reason,
				"status":      status,
				"created_at":  createdAt,
				"activity_at": activityAt,
			})
		}
		stmt.Totals["count"] = int64(len(stmt.Rows))
		stmt.Totals["refund_amount"] = total
	case ReportTypeDisputes:
		rows, err := r.db.Query(ctx, `
SELECT id, payment_id, status, reason, amount, currency, created_at, COALESCE(resolved_at, created_at)
FROM paygate_disputes.disputes
WHERE merchant_id = $1 AND created_at >= $2 AND created_at < $3
ORDER BY created_at DESC
`, merchantID, start, end)
		if err != nil {
			return Statement{}, err
		}
		defer rows.Close()
		var total int64
		for rows.Next() {
			var id, paymentID, status, reason, currency string
			var amount int64
			var createdAt, activityAt time.Time
			if err := rows.Scan(&id, &paymentID, &status, &reason, &amount, &currency, &createdAt, &activityAt); err != nil {
				return Statement{}, err
			}
			total += amount
			stmt.Rows = append(stmt.Rows, map[string]any{
				"id":          id,
				"payment_id":  paymentID,
				"status":      status,
				"reason":      reason,
				"amount":      amount,
				"currency":    currency,
				"created_at":  createdAt,
				"activity_at": activityAt,
			})
		}
		stmt.Totals["count"] = int64(len(stmt.Rows))
		stmt.Totals["dispute_amount"] = total
	case ReportTypePayouts:
		rows, err := r.db.Query(ctx, `
SELECT id, settlement_id, amount, currency, status, approval_status, created_at, COALESCE(completed_at, created_at)
FROM paygate_payouts.payouts
WHERE merchant_id = $1 AND created_at >= $2 AND created_at < $3
ORDER BY created_at DESC
`, merchantID, start, end)
		if err != nil {
			return Statement{}, err
		}
		defer rows.Close()
		var total int64
		for rows.Next() {
			var id, settlementID, currency, status, approvalStatus string
			var amount int64
			var createdAt, activityAt time.Time
			if err := rows.Scan(&id, &settlementID, &amount, &currency, &status, &approvalStatus, &createdAt, &activityAt); err != nil {
				return Statement{}, err
			}
			total += amount
			stmt.Rows = append(stmt.Rows, map[string]any{
				"id":              id,
				"settlement_id":   settlementID,
				"amount":          amount,
				"currency":        currency,
				"status":          status,
				"approval_status": approvalStatus,
				"created_at":      createdAt,
				"activity_at":     activityAt,
			})
		}
		stmt.Totals["count"] = int64(len(stmt.Rows))
		stmt.Totals["payout_amount"] = total
	case ReportTypeSettlements:
		rows, err := r.db.Query(ctx, `
SELECT id, gross_net_amount, reserve_amount, net_amount, currency, status, payment_count, created_at, COALESCE(processed_at, created_at)
FROM paygate_settlements.settlements
WHERE merchant_id = $1 AND created_at >= $2 AND created_at < $3
ORDER BY created_at DESC
`, merchantID, start, end)
		if err != nil {
			return Statement{}, err
		}
		defer rows.Close()
		var gross, reserve, net int64
		for rows.Next() {
			var id, currency, status string
			var grossNet, reserveAmount, netAmount int64
			var paymentCount int
			var createdAt, activityAt time.Time
			if err := rows.Scan(&id, &grossNet, &reserveAmount, &netAmount, &currency, &status, &paymentCount, &createdAt, &activityAt); err != nil {
				return Statement{}, err
			}
			gross += grossNet
			reserve += reserveAmount
			net += netAmount
			stmt.Rows = append(stmt.Rows, map[string]any{
				"id":             id,
				"gross_net":      grossNet,
				"reserve_amount": reserveAmount,
				"net_amount":     netAmount,
				"currency":       currency,
				"status":         status,
				"payment_count":  paymentCount,
				"created_at":     createdAt,
				"activity_at":    activityAt,
			})
		}
		stmt.Totals["count"] = int64(len(stmt.Rows))
		stmt.Totals["gross_net_amount"] = gross
		stmt.Totals["reserve_amount"] = reserve
		stmt.Totals["net_amount"] = net
	case ReportTypeTaxSummary:
		base, err := r.BuildStatement(ctx, merchantID, ReportTypePayments, start, end)
		if err != nil {
			return Statement{}, err
		}
		stmt.Totals["taxable_fee_amount"] = base.Totals["fee_amount"]
		stmt.Totals["tax_amount"] = calculateTax(base.Totals["fee_amount"], profile.DefaultTaxRateBPS)
		stmt.Totals["gross_amount"] = base.Totals["gross_amount"]
		stmt.Totals["payment_count"] = base.Totals["count"]
		stmt.Rows = append(stmt.Rows, map[string]any{
			"legal_name":          profile.LegalName,
			"gstin":               profile.GSTIN,
			"business_state_code": profile.BusinessStateCode,
			"place_of_supply":     profile.PlaceOfSupply,
			"tax_rate_bps":        profile.DefaultTaxRateBPS,
		})
	case ReportTypeRecon:
		rows, err := r.db.Query(ctx, `
SELECT id, mismatch_type, entity_type, entity_id, expected_value, actual_value, description, status, COALESCE(assigned_to, ''), COALESCE(resolution_code, ''), created_at
FROM paygate_recon.recon_mismatches
WHERE merchant_id = $1 AND created_at >= $2 AND created_at < $3
ORDER BY created_at DESC
`, merchantID, start, end)
		if err != nil {
			return Statement{}, err
		}
		defer rows.Close()
		for rows.Next() {
			var id, mismatchType, entityType, entityID, expectedValue, actualValue, description, status, assignedTo, resolutionCode string
			var createdAt time.Time
			if err := rows.Scan(&id, &mismatchType, &entityType, &entityID, &expectedValue, &actualValue, &description, &status, &assignedTo, &resolutionCode, &createdAt); err != nil {
				return Statement{}, err
			}
			stmt.Rows = append(stmt.Rows, map[string]any{
				"id":              id,
				"mismatch_type":   mismatchType,
				"entity_type":     entityType,
				"entity_id":       entityID,
				"expected_value":  expectedValue,
				"actual_value":    actualValue,
				"description":     description,
				"status":          status,
				"assigned_to":     assignedTo,
				"resolution_code": resolutionCode,
				"created_at":      createdAt,
			})
		}
		stmt.Totals["count"] = int64(len(stmt.Rows))
	default:
		return Statement{}, fmt.Errorf("unsupported report type %q", reportType)
	}
	return stmt, nil
}

func (r *Repository) CreateExportJob(ctx context.Context, req ExportRequest, stmt Statement) (ExportJob, error) {
	format, ok := normalizeExportFormat(req.Format)
	if !ok {
		return ExportJob{}, ErrUnsupportedExportFormat
	}
	content, err := statementCSV(stmt)
	if err != nil {
		return ExportJob{}, err
	}
	job := ExportJob{
		ID:                idgen.New("rpt"),
		MerchantID:        req.MerchantID,
		ReportType:        req.ReportType,
		Format:            format,
		Status:            ExportStatusCompleted,
		FileName:          fmt.Sprintf("%s-%s-%s.%s", req.ReportType, req.PeriodStart.Format("20060102"), req.PeriodEnd.Format("20060102"), format),
		ContentType:       "text/csv",
		FileSizeBytes:     int64(len(content)),
		ContentText:       content,
		DownloadToken:     idgen.New("dl"),
		DownloadExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		CreatedAt:         time.Now().UTC(),
	}
	job.CompletedAt = &job.CreatedAt
	job.Filters = map[string]any{
		"period_start": req.PeriodStart.Unix(),
		"period_end":   req.PeriodEnd.Unix(),
		"report_type":  req.ReportType,
	}
	filtersJSON, _ := json.Marshal(job.Filters)
	_, err = r.db.Exec(ctx, `
INSERT INTO paygate_reporting.export_jobs
    (id, merchant_id, report_type, format, status, file_name, content_type, file_size_bytes, filters_json, content_text, download_token, download_expires_at, created_at, completed_at, error_message)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12,$13,$14,$15)
`, job.ID, job.MerchantID, job.ReportType, job.Format, job.Status, job.FileName, job.ContentType, job.FileSizeBytes, string(filtersJSON), job.ContentText, job.DownloadToken, job.DownloadExpiresAt, job.CreatedAt, job.CompletedAt, job.ErrorMessage)
	if err != nil {
		return ExportJob{}, fmt.Errorf("insert export job: %w", err)
	}
	return job, nil
}

func (r *Repository) ListExportJobs(ctx context.Context, merchantID string, limit int) ([]ExportJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, report_type, format, status, file_name, content_type, file_size_bytes, filters_json, content_text,
       download_token, download_expires_at, error_message, created_at, completed_at
FROM paygate_reporting.export_jobs
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2
`, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExportJobs(rows)
}

func (r *Repository) GetExportJob(ctx context.Context, merchantID, exportID string) (ExportJob, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, report_type, format, status, file_name, content_type, file_size_bytes, filters_json, content_text,
       download_token, download_expires_at, error_message, created_at, completed_at
FROM paygate_reporting.export_jobs
WHERE merchant_id = $1 AND id = $2
`, merchantID, exportID)
	if err != nil {
		return ExportJob{}, err
	}
	defer rows.Close()
	items, err := scanExportJobs(rows)
	if err != nil {
		return ExportJob{}, err
	}
	if len(items) == 0 {
		return ExportJob{}, pgx.ErrNoRows
	}
	return items[0], nil
}

func scanExportJobs(rows pgx.Rows) ([]ExportJob, error) {
	var out []ExportJob
	for rows.Next() {
		var item ExportJob
		var reportType, status string
		var filtersRaw []byte
		if err := rows.Scan(&item.ID, &item.MerchantID, &reportType, &item.Format, &status, &item.FileName, &item.ContentType, &item.FileSizeBytes, &filtersRaw, &item.ContentText, &item.DownloadToken, &item.DownloadExpiresAt, &item.ErrorMessage, &item.CreatedAt, &item.CompletedAt); err != nil {
			return nil, err
		}
		item.ReportType = ReportType(reportType)
		item.Status = ExportStatus(status)
		_ = json.Unmarshal(filtersRaw, &item.Filters)
		out = append(out, item)
	}
	return out, rows.Err()
}

func statementCSV(stmt Statement) (string, error) {
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write([]string{"entity_type", stmt.EntityType}); err != nil {
		return "", err
	}
	if err := w.Write([]string{"period_start", stmt.PeriodStart.Format(time.RFC3339)}); err != nil {
		return "", err
	}
	if err := w.Write([]string{"period_end", stmt.PeriodEnd.Format(time.RFC3339)}); err != nil {
		return "", err
	}
	if len(stmt.Totals) > 0 {
		if err := w.Write([]string{"section", "totals"}); err != nil {
			return "", err
		}
		keys := make([]string, 0, len(stmt.Totals))
		for k := range stmt.Totals {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := w.Write([]string{k, strconv.FormatInt(stmt.Totals[k], 10)}); err != nil {
				return "", err
			}
		}
	}
	if len(stmt.Rows) > 0 {
		headers := make([]string, 0, len(stmt.Rows[0]))
		for k := range stmt.Rows[0] {
			headers = append(headers, k)
		}
		sort.Strings(headers)
		if err := w.Write(headers); err != nil {
			return "", err
		}
		for _, row := range stmt.Rows {
			record := make([]string, 0, len(headers))
			for _, header := range headers {
				record = append(record, stringify(row[header]))
			}
			if err := w.Write(record); err != nil {
				return "", err
			}
		}
	}
	w.Flush()
	return b.String(), w.Error()
}

func calculateTax(amount int64, bps int) int64 {
	return amount * int64(bps) / 10000
}

func stringify(v any) string {
	switch tv := v.(type) {
	case nil:
		return ""
	case string:
		return tv
	case int:
		return strconv.Itoa(tv)
	case int64:
		return strconv.FormatInt(tv, 10)
	case bool:
		return strconv.FormatBool(tv)
	case time.Time:
		return tv.Format(time.RFC3339)
	default:
		raw, _ := json.Marshal(tv)
		return string(raw)
	}
}
