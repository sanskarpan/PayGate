package payout

import (
	"errors"
	"strings"
	"time"
)

type BeneficiaryDestinationType string
type BeneficiaryStatus string
type ApprovalStatus string

const (
	DestinationTypeBankAccount BeneficiaryDestinationType = "bank_account"
	DestinationTypeVPA         BeneficiaryDestinationType = "vpa"
)

const (
	BeneficiaryStatusPendingVerification BeneficiaryStatus = "pending_verification"
	BeneficiaryStatusVerified            BeneficiaryStatus = "verified"
	BeneficiaryStatusApproved            BeneficiaryStatus = "approved"
	BeneficiaryStatusRejected            BeneficiaryStatus = "rejected"
	BeneficiaryStatusDisabled            BeneficiaryStatus = "disabled"
)

const (
	ApprovalStatusNotRequired ApprovalStatus = "not_required"
	ApprovalStatusPending     ApprovalStatus = "pending"
	ApprovalStatusApproved    ApprovalStatus = "approved"
	ApprovalStatusRejected    ApprovalStatus = "rejected"
)

var (
	ErrBeneficiaryNotFound    = errors.New("beneficiary not found")
	ErrBeneficiaryNotApproved = errors.New("beneficiary must be approved before payout")
	ErrBeneficiaryInvalid     = errors.New("invalid beneficiary payload")
	ErrPayoutApprovalRequired = errors.New("payout approval is required before execution")
	ErrPayoutApprovalRejected = errors.New("payout approval was rejected")
	ErrPayoutBatchNotFound    = errors.New("payout batch not found")
)

type Beneficiary struct {
	ID                     string
	MerchantID             string
	DestinationType        BeneficiaryDestinationType
	AccountHolderName      string
	BankAccountLast4       string
	BankIFSC               string
	VPA                    string
	Fingerprint            string
	Status                 BeneficiaryStatus
	VerificationFreshUntil *time.Time
	ApprovedAt             *time.Time
	ApprovalNotes          string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type BeneficiaryVerification struct {
	ID                string
	BeneficiaryID     string
	MerchantID        string
	Provider          string
	ProviderReference string
	Status            string
	Evidence          map[string]any
	VerifiedAt        *time.Time
	CreatedAt         time.Time
}

type ApprovalRecord struct {
	ID         string
	PayoutID   string
	MerchantID string
	Actor      string
	ActorScope string
	Decision   string
	Notes      string
	CreatedAt  time.Time
}

type Batch struct {
	ID             string
	MerchantID     string
	DryRun         bool
	Status         string
	IdempotencyKey string
	Summary        map[string]any
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type BatchItem struct {
	ID            string
	BatchID       string
	MerchantID    string
	SettlementID  string
	BeneficiaryID string
	PayoutID      string
	Amount        int64
	Currency      string
	Status        string
	ErrorText     string
	CreatedAt     time.Time
}

func (b Beneficiary) Validate() error {
	if strings.TrimSpace(b.AccountHolderName) == "" {
		return ErrBeneficiaryInvalid
	}
	switch b.DestinationType {
	case DestinationTypeBankAccount:
		if strings.TrimSpace(b.BankAccountLast4) == "" || strings.TrimSpace(b.BankIFSC) == "" {
			return ErrBeneficiaryInvalid
		}
	case DestinationTypeVPA:
		if strings.TrimSpace(b.VPA) == "" {
			return ErrBeneficiaryInvalid
		}
	default:
		return ErrBeneficiaryInvalid
	}
	return nil
}
