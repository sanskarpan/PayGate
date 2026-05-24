package reporting

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrExportNotFound = errors.New("export not found")

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Catalog() []CatalogItem {
	return []CatalogItem{
		{ReportType: ReportTypePayments, Label: "Payments", Description: "Payment and fee statement", SupportsAPIs: true},
		{ReportType: ReportTypeRefunds, Label: "Refunds", Description: "Refund operational statement", SupportsAPIs: true},
		{ReportType: ReportTypeDisputes, Label: "Disputes", Description: "Dispute lifecycle statement", SupportsAPIs: true},
		{ReportType: ReportTypePayouts, Label: "Payouts", Description: "Payout and approval statement", SupportsAPIs: true},
		{ReportType: ReportTypeSettlements, Label: "Settlements", Description: "Settlement and reserve statement", SupportsAPIs: true},
		{ReportType: ReportTypeTaxSummary, Label: "Tax summary", Description: "GST/taxable fee summary", SupportsAPIs: true},
		{ReportType: ReportTypeRecon, Label: "Reconciliation mismatches", Description: "Mismatch and resolution export", SupportsAPIs: true},
	}
}

func (s *Service) GetTaxProfile(ctx context.Context, merchantID string) (TaxProfile, error) {
	return s.repo.GetTaxProfile(ctx, merchantID)
}

func (s *Service) UpsertTaxProfile(ctx context.Context, in TaxProfile) (TaxProfile, error) {
	return s.repo.UpsertTaxProfile(ctx, in)
}

func (s *Service) BuildStatement(ctx context.Context, merchantID string, reportType ReportType, start, end time.Time) (Statement, error) {
	return s.repo.BuildStatement(ctx, merchantID, reportType, start, end)
}

func (s *Service) RequestExport(ctx context.Context, req ExportRequest) (ExportJob, error) {
	stmt, err := s.repo.BuildStatement(ctx, req.MerchantID, req.ReportType, req.PeriodStart, req.PeriodEnd)
	if err != nil {
		return ExportJob{}, err
	}
	return s.repo.CreateExportJob(ctx, req, stmt)
}

func (s *Service) ListExports(ctx context.Context, merchantID string, limit int) ([]ExportJob, error) {
	return s.repo.ListExportJobs(ctx, merchantID, limit)
}

func (s *Service) GetExport(ctx context.Context, merchantID, exportID string) (ExportJob, error) {
	job, err := s.repo.GetExportJob(ctx, merchantID, exportID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ExportJob{}, ErrExportNotFound
	}
	return job, err
}
