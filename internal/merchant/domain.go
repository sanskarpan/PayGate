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

const (
	MerchantStatusActive      MerchantStatus = "active"
	MerchantStatusSuspended   MerchantStatus = "suspended"
	MerchantStatusDeactivated MerchantStatus = "deactivated"
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
	ErrInvalidMerchantName  = errors.New("merchant name is required and must be under 255 characters")
	ErrInvalidMerchantEmail = errors.New("merchant email must be a valid email address")
	ErrInvalidBusinessType  = errors.New("merchant business_type is required")
	ErrMerchantSuspended    = errors.New("merchant is suspended")
	ErrMerchantDeactivated  = errors.New("merchant is deactivated")
	ErrInvalidAPIKeyMode    = errors.New("invalid api key mode")
	ErrInvalidAPIKeyScope   = errors.New("invalid api key scope")
	ErrAPIKeyNotActive      = errors.New("api key is not active")
)

type Merchant struct {
	ID           string
	Name         string
	Email        string
	BusinessType string
	Status       MerchantStatus
	Settings     map[string]any
	CreatedAt    time.Time
	UpdatedAt    time.Time
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

func (m Merchant) CanIssueAPIKey() error {
	switch m.Status {
	case MerchantStatusActive:
		return nil
	case MerchantStatusSuspended:
		return ErrMerchantSuspended
	case MerchantStatusDeactivated:
		return ErrMerchantDeactivated
	default:
		return ErrMerchantDeactivated
	}
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
