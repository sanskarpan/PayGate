package merchant

import (
	"errors"
	"strings"
	"time"
)

type OnboardingPartyType string
type VerificationStatus string
type DocumentStatus string
type ScreeningStatus string
type CapabilityStatus string
type CapabilityCode string
type ReservePolicyType string
type ReserveEscalationStatus string

const (
	PartyTypeBeneficialOwner OnboardingPartyType = "beneficial_owner"
	PartyTypeController      OnboardingPartyType = "controller"
)

const (
	VerificationStatusPending  VerificationStatus = "pending"
	VerificationStatusVerified VerificationStatus = "verified"
	VerificationStatusRejected VerificationStatus = "rejected"
)

const (
	DocumentStatusRequested DocumentStatus = "requested"
	DocumentStatusUploaded  DocumentStatus = "uploaded"
	DocumentStatusApproved  DocumentStatus = "approved"
	DocumentStatusRejected  DocumentStatus = "rejected"
	DocumentStatusExpired   DocumentStatus = "expired"
)

const (
	ScreeningStatusPending ScreeningStatus = "pending"
	ScreeningStatusPassed  ScreeningStatus = "passed"
	ScreeningStatusReview  ScreeningStatus = "review"
	ScreeningStatusFailed  ScreeningStatus = "failed"
)

const (
	CapabilityStatusEnabled    CapabilityStatus = "enabled"
	CapabilityStatusRestricted CapabilityStatus = "restricted"
	CapabilityStatusDisabled   CapabilityStatus = "disabled"
)

const (
	CapabilityPayments CapabilityCode = "payments"
	CapabilityRefunds  CapabilityCode = "refunds"
	CapabilityPayouts  CapabilityCode = "payouts"
	CapabilityUPI      CapabilityCode = "upi"
	CapabilityCards    CapabilityCode = "cards"
)

const (
	ReservePolicyNone              ReservePolicyType = "none"
	ReservePolicyFixedPercentage   ReservePolicyType = "fixed_percentage"
	ReservePolicyRollingPercentage ReservePolicyType = "rolling_percentage"
)

const (
	ReserveEscalationPending  ReserveEscalationStatus = "pending"
	ReserveEscalationApproved ReserveEscalationStatus = "approved"
	ReserveEscalationRejected ReserveEscalationStatus = "rejected"
)

var (
	ErrOnboardingOwnersIncomplete    = errors.New("merchant onboarding requires at least one verified beneficial owner and one verified controller")
	ErrOnboardingDocumentsIncomplete = errors.New("merchant onboarding requires approved supporting documents")
	ErrOnboardingScreeningIncomplete = errors.New("merchant onboarding requires a completed screening pass")
	ErrCapabilityRestricted          = errors.New("merchant capability is restricted")
	ErrInvalidCapability             = errors.New("invalid merchant capability")
	ErrInvalidDocumentState          = errors.New("invalid onboarding document state transition")
	ErrInvalidPartyData              = errors.New("invalid onboarding party data")
	ErrInvalidReservePolicy          = errors.New("invalid reserve policy")
	ErrReserveEscalationNotFound     = errors.New("reserve escalation not found")
)

type OnboardingParty struct {
	ID                 string
	ApplicationID      string
	MerchantID         string
	PartyType          OnboardingPartyType
	FullName           string
	Title              string
	Email              string
	Phone              string
	OwnershipBPS       int
	VerificationStatus VerificationStatus
	EvidenceNotes      string
	Revision           int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type OnboardingDocument struct {
	ID            string
	ApplicationID string
	MerchantID    string
	DocumentType  string
	FileName      string
	ContentType   string
	StorageKey    string
	RequestReason string
	ReviewNotes   string
	Status        DocumentStatus
	RequestedAt   time.Time
	UploadedAt    *time.Time
	ReviewedAt    *time.Time
	ExpiresAt     *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ScreeningCase struct {
	ID                string
	ApplicationID     string
	MerchantID        string
	ScreeningType     string
	Provider          string
	ProviderReference string
	SubjectName       string
	Status            ScreeningStatus
	ResultPayload     map[string]any
	ReviewedBy        string
	ScreenedAt        time.Time
	ReviewedAt        *time.Time
	CreatedAt         time.Time
}

type MerchantCapability struct {
	ID             string
	MerchantID     string
	CapabilityCode CapabilityCode
	Status         CapabilityStatus
	Reason         string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ReservePolicy struct {
	ID              string
	MerchantID      string
	PolicyType      ReservePolicyType
	PercentageBPS   int
	HoldDays        int
	ThresholdAmount int64
	Notes           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ReserveEscalation struct {
	ID                       string
	MerchantID               string
	RiskEventID              string
	TriggerScore             int
	TriggeredRules           []string
	Status                   ReserveEscalationStatus
	SuggestedPolicyType      ReservePolicyType
	SuggestedPercentageBPS   int
	SuggestedHoldDays        int
	SuggestedThresholdAmount int64
	Rationale                string
	ReviewNotes              string
	ReviewedBy               string
	ReviewedAt               *time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

func DefaultCapabilities(merchantID string) []MerchantCapability {
	now := time.Now().UTC()
	return []MerchantCapability{
		{MerchantID: merchantID, CapabilityCode: CapabilityPayments, Status: CapabilityStatusEnabled, CreatedAt: now, UpdatedAt: now},
		{MerchantID: merchantID, CapabilityCode: CapabilityRefunds, Status: CapabilityStatusEnabled, CreatedAt: now, UpdatedAt: now},
		{MerchantID: merchantID, CapabilityCode: CapabilityPayouts, Status: CapabilityStatusEnabled, CreatedAt: now, UpdatedAt: now},
		{MerchantID: merchantID, CapabilityCode: CapabilityUPI, Status: CapabilityStatusEnabled, CreatedAt: now, UpdatedAt: now},
		{MerchantID: merchantID, CapabilityCode: CapabilityCards, Status: CapabilityStatusEnabled, CreatedAt: now, UpdatedAt: now},
	}
}

func DefaultReservePolicy(merchantID string) ReservePolicy {
	return ReservePolicy{
		MerchantID: merchantID,
		PolicyType: ReservePolicyNone,
	}
}

func (p OnboardingParty) Validate() error {
	if strings.TrimSpace(p.FullName) == "" {
		return ErrInvalidPartyData
	}
	switch p.PartyType {
	case PartyTypeBeneficialOwner, PartyTypeController:
	default:
		return ErrInvalidPartyData
	}
	if p.PartyType == PartyTypeBeneficialOwner && p.OwnershipBPS <= 0 {
		return ErrInvalidPartyData
	}
	switch p.VerificationStatus {
	case "", VerificationStatusPending, VerificationStatusVerified, VerificationStatusRejected:
	default:
		return ErrInvalidPartyData
	}
	return nil
}

func (d OnboardingDocument) IsUsable(now time.Time) bool {
	if d.Status != DocumentStatusApproved {
		return false
	}
	if d.ExpiresAt != nil && !d.ExpiresAt.After(now) {
		return false
	}
	return true
}

func ValidateCapabilityCode(code CapabilityCode) error {
	switch code {
	case CapabilityPayments, CapabilityRefunds, CapabilityPayouts, CapabilityUPI, CapabilityCards:
		return nil
	default:
		return ErrInvalidCapability
	}
}

func (p ReservePolicy) Validate() error {
	switch p.PolicyType {
	case ReservePolicyNone:
		if p.PercentageBPS != 0 || p.HoldDays != 0 {
			return ErrInvalidReservePolicy
		}
	case ReservePolicyFixedPercentage, ReservePolicyRollingPercentage:
		if p.PercentageBPS < 0 || p.PercentageBPS > 10000 || p.HoldDays < 0 {
			return ErrInvalidReservePolicy
		}
	default:
		return ErrInvalidReservePolicy
	}
	if p.ThresholdAmount < 0 {
		return ErrInvalidReservePolicy
	}
	return nil
}

func (p ReservePolicy) CalculateReserve(netAmount int64) int64 {
	if netAmount <= 0 {
		return 0
	}
	if p.ThresholdAmount > 0 && netAmount < p.ThresholdAmount {
		return 0
	}
	switch p.PolicyType {
	case ReservePolicyFixedPercentage, ReservePolicyRollingPercentage:
		return (netAmount * int64(p.PercentageBPS)) / 10000
	default:
		return 0
	}
}
