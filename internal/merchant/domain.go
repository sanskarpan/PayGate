package merchant

import (
	"errors"
	"net/mail"
	"strings"
	"time"
)

type MerchantStatus string

type APIKeyMode string

type APIKeyScope string

type APIKeyStatus string
type OnboardingState string

const (
	MerchantStatusActive      MerchantStatus = "active"
	MerchantStatusSuspended   MerchantStatus = "suspended"
	MerchantStatusDeactivated MerchantStatus = "deactivated"
)

const (
	OnboardingStateDraft            OnboardingState = "draft"
	OnboardingStateSubmitted        OnboardingState = "submitted"
	OnboardingStateInReview         OnboardingState = "in_review"
	OnboardingStateNeedsInformation OnboardingState = "needs_information"
	OnboardingStateApproved         OnboardingState = "approved"
	OnboardingStateRejected         OnboardingState = "rejected"
)

const (
	APIKeyModeTest APIKeyMode = "test"
	APIKeyModeLive APIKeyMode = "live"
)

const (
	APIKeyScopeRead  APIKeyScope = "read"
	APIKeyScopeWrite APIKeyScope = "write"
	APIKeyScopeAdmin APIKeyScope = "admin"
)

const (
	APIKeyStatusActive  APIKeyStatus = "active"
	APIKeyStatusRevoked APIKeyStatus = "revoked"
)

var (
	ErrInvalidMerchantName           = errors.New("merchant name is required and must be under 255 characters")
	ErrInvalidMerchantEmail          = errors.New("merchant email must be a valid email address")
	ErrInvalidBusinessType           = errors.New("merchant business_type is required")
	ErrMerchantSuspended             = errors.New("merchant is suspended")
	ErrMerchantDeactivated           = errors.New("merchant is deactivated")
	ErrInvalidAPIKeyMode             = errors.New("invalid api key mode")
	ErrInvalidAPIKeyScope            = errors.New("invalid api key scope")
	ErrAPIKeyNotActive               = errors.New("api key is not active")
	ErrOnboardingNotApproved         = errors.New("merchant onboarding is not approved for live capabilities")
	ErrOnboardingApplicationNotFound = errors.New("merchant onboarding application not found")
	ErrInvalidOnboardingState        = errors.New("invalid onboarding application state transition")
	ErrInvalidOnboardingData         = errors.New("merchant onboarding application is incomplete")
)

type Merchant struct {
	ID               string
	Name             string
	Email            string
	BusinessType     string
	Status           MerchantStatus
	Settings         map[string]any
	OnboardingStatus OnboardingState
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type OnboardingApplication struct {
	ID                     string
	MerchantID             string
	LegalName              string
	BusinessClassification string
	RegistrationNumber     string
	TaxIdentifier          string
	CountryCode            string
	State                  OnboardingState
	ReviewerNotes          string
	SubmittedAt            *time.Time
	ReviewedAt             *time.Time
	ApprovedAt             *time.Time
	RejectedAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type APIKey struct {
	ID         string
	MerchantID string
	SecretHash string
	Mode       APIKeyMode
	Scope      APIKeyScope
	Status     APIKeyStatus
	AllowedIPs []string
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

func (m Merchant) ValidateForCreate() error {
	name := strings.TrimSpace(m.Name)
	if name == "" || len(name) > 255 {
		return ErrInvalidMerchantName
	}
	email := strings.TrimSpace(m.Email)
	if email == "" {
		return ErrInvalidMerchantEmail
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return ErrInvalidMerchantEmail
	}
	if strings.TrimSpace(m.BusinessType) == "" {
		return ErrInvalidBusinessType
	}
	return nil
}

func (m Merchant) CanIssueAPIKey(mode APIKeyMode) error {
	switch m.Status {
	case MerchantStatusActive:
		if mode == APIKeyModeLive && m.OnboardingStatus != OnboardingStateApproved {
			return ErrOnboardingNotApproved
		}
		return nil
	case MerchantStatusSuspended:
		return ErrMerchantSuspended
	case MerchantStatusDeactivated:
		return ErrMerchantDeactivated
	default:
		return ErrMerchantDeactivated
	}
}

func (a OnboardingApplication) ValidateForSubmission() error {
	if strings.TrimSpace(a.LegalName) == "" {
		return ErrInvalidOnboardingData
	}
	if strings.TrimSpace(a.BusinessClassification) == "" {
		return ErrInvalidOnboardingData
	}
	if strings.TrimSpace(a.RegistrationNumber) == "" {
		return ErrInvalidOnboardingData
	}
	if strings.TrimSpace(a.TaxIdentifier) == "" {
		return ErrInvalidOnboardingData
	}
	return nil
}

func ValidateMode(mode APIKeyMode) error {
	switch mode {
	case APIKeyModeTest, APIKeyModeLive:
		return nil
	default:
		return ErrInvalidAPIKeyMode
	}
}

func ValidateScope(scope APIKeyScope) error {
	switch scope {
	case APIKeyScopeRead, APIKeyScopeWrite, APIKeyScopeAdmin:
		return nil
	default:
		return ErrInvalidAPIKeyScope
	}
}

func ScopeAllows(granted, required APIKeyScope) bool {
	if required == "" {
		return true
	}
	order := map[APIKeyScope]int{
		APIKeyScopeRead:  1,
		APIKeyScopeWrite: 2,
		APIKeyScopeAdmin: 3,
	}
	return order[granted] >= order[required]
}
