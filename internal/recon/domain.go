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
	ID            string
	BatchID       string
	MerchantID    string
	MismatchType  MismatchType
	EntityType    string
	EntityID      string
	ExpectedValue string
	ActualValue   string
	Description   string
	Resolved      bool
	CreatedAt     time.Time
}
