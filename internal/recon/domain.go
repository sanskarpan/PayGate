package recon

import (
	"errors"
	"time"
)

// BatchType classifies the reconciliation run.
type BatchType string

const (
	BatchTypeLedgerBalance BatchType = "ledger_balance"
	BatchTypePaymentLedger BatchType = "payment_ledger"
	BatchTypeThreeWay      BatchType = "three_way"
)

// MismatchType classifies a detected discrepancy.
type MismatchType string

const (
	// LedgerImbalance: total debits ≠ total credits for a merchant.
	MismatchLedgerImbalance MismatchType = "ledger_imbalance"
	// PaymentMissingLedger: a captured payment has no matching ledger entries.
	MismatchPaymentMissingLedger MismatchType = "payment_missing_ledger"
	// PaymentAmountMismatch: ledger sum for a payment differs from payment.amount.
	MismatchPaymentAmountMismatch MismatchType = "payment_amount_mismatch"
	// SettlementPaymentMismatch: a settled payment's amount doesn't match the settlement item.
	MismatchSettlementPaymentMismatch MismatchType = "settlement_payment_mismatch"
	// PaymentSettledNotInBatch: payment.settled=true but no settlement_item found.
	MismatchPaymentSettledNotInBatch MismatchType = "payment_settled_not_in_batch"
	// OrphanSettlementItem: a settlement item points to a missing payment.
	MismatchOrphanSettlementItem MismatchType = "orphan_settlement_item"
	// ExternalSourceMissingInternal: imported external source row has no matching internal entity.
	MismatchExternalSourceMissingInternal MismatchType = "external_source_missing_internal"
	// ExternalSourceAmountMismatch: imported external amount differs from internal amount.
	MismatchExternalSourceAmountMismatch MismatchType = "external_source_amount_mismatch"
	// InternalMissingExternalSource: internal entity was not present in the imported source set.
	MismatchInternalMissingExternalSource MismatchType = "internal_missing_external_source"
)

var ErrBatchNotFound = errors.New("recon batch not found")

// ReconBatch records the outcome of a reconciliation run.
type ReconBatch struct {
	ID            string
	MerchantID    string
	BatchType     BatchType
	Status        string
	PeriodStart   time.Time
	PeriodEnd     time.Time
	CheckedCount  int
	MismatchCount int
	ErrorMessage  string
	CreatedAt     time.Time
}

// ReconMismatch records a single detected discrepancy.
type ReconMismatch struct {
	ID              string
	BatchID         string
	MerchantID      string
	SourceImportID  string
	MismatchType    MismatchType
	EntityType      string
	EntityID        string
	ExpectedValue   string
	ActualValue     string
	Description     string
	Resolved        bool
	Status          string
	AssignedTo      string
	AssignedAt      *time.Time
	ResolvedAt      *time.Time
	ResolvedBy      string
	ResolutionCode  string
	ResolutionNotes string
	CreatedAt       time.Time
}

type SourceImport struct {
	ID            string
	BatchID       string
	MerchantID    string
	SourceType    string
	Status        string
	PeriodStart   time.Time
	PeriodEnd     time.Time
	EntryCount    int
	MismatchCount int
	CreatedAt     time.Time
}

type SourceEntry struct {
	ID             string
	SourceImportID string
	MerchantID     string
	EntityType     string
	ExternalID     string
	ReferenceID    string
	Amount         int64
	Currency       string
	Status         string
	OccurredAt     time.Time
	Metadata       map[string]any
}

type MismatchNote struct {
	ID         string
	MismatchID string
	MerchantID string
	Author     string
	Note       string
	CreatedAt  time.Time
}

type ImportSourceInput struct {
	MerchantID  string
	SourceType  string
	PeriodStart time.Time
	PeriodEnd   time.Time
	Entries     []SourceEntry
}
