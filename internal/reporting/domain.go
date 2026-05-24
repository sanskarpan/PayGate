package reporting

import "time"

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
