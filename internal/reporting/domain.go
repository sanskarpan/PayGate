package reporting

import (
	"strings"
	"time"
)

// ExportFormatCSV is the only format the export engine renders. CreateExportJob
// always calls statementCSV and always serves text/csv, so accepting any other
// value would hand back a file whose extension contradicts its contents.
const ExportFormatCSV = "csv"

// normalizeExportFormat lowercases and defaults a requested export format,
// reporting false when the caller asked for something we cannot render.
func normalizeExportFormat(format string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(format))
	if normalized == "" {
		return ExportFormatCSV, true
	}
	if normalized != ExportFormatCSV {
		return "", false
	}
	return normalized, true
}

type ExportStatus string

const (
	ExportStatusPending   ExportStatus = "pending"
	ExportStatusCompleted ExportStatus = "completed"
	ExportStatusFailed    ExportStatus = "failed"
)

type ReportType string

const (
	ReportTypePayments    ReportType = "payments"
	ReportTypeRefunds     ReportType = "refunds"
	ReportTypeDisputes    ReportType = "disputes"
	ReportTypePayouts     ReportType = "payouts"
	ReportTypeSettlements ReportType = "settlements"
	ReportTypeTaxSummary  ReportType = "tax_summary"
	ReportTypeRecon       ReportType = "recon_mismatches"
)

type CatalogItem struct {
	ReportType   ReportType `json:"report_type"`
	Label        string     `json:"label"`
	Description  string     `json:"description"`
	SupportsAPIs bool       `json:"supports_api"`
}

type ExportJob struct {
	ID                string
	MerchantID        string
	ReportType        ReportType
	Format            string
	Status            ExportStatus
	FileName          string
	ContentType       string
	FileSizeBytes     int64
	Filters           map[string]any
	ContentText       string
	DownloadToken     string
	DownloadExpiresAt time.Time
	ErrorMessage      string
	CreatedAt         time.Time
	CompletedAt       *time.Time
}

type TaxProfile struct {
	MerchantID        string
	LegalName         string
	GSTIN             string
	BusinessStateCode string
	PlaceOfSupply     string
	DefaultTaxRateBPS int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Statement struct {
	EntityType  string
	PeriodStart time.Time
	PeriodEnd   time.Time
	TaxRateBPS  int
	Totals      map[string]int64
	Rows        []map[string]any
}

type ExportRequest struct {
	MerchantID  string
	ReportType  ReportType
	Format      string
	PeriodStart time.Time
	PeriodEnd   time.Time
}
