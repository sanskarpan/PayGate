package merchant

import (
	"errors"
	"strings"
	"time"
)

type MerchantUserRole string

type MerchantUserStatus string

const (
	MerchantUserRoleAdmin     MerchantUserRole = "admin"
	MerchantUserRoleDeveloper MerchantUserRole = "developer"
	MerchantUserRoleReadonly  MerchantUserRole = "readonly"
	MerchantUserRoleOps       MerchantUserRole = "ops"
)

const (
	MerchantUserStatusActive    MerchantUserStatus = "active"
	MerchantUserStatusSuspended MerchantUserStatus = "suspended"
)

var (
	ErrMerchantUserNotFound   = errors.New("merchant user not found")
	ErrMerchantUserNotActive  = errors.New("merchant user is not active")
	ErrInvalidMerchantUser    = errors.New("merchant user email is required")
	ErrInvalidMerchantPass    = errors.New("merchant user password must be at least 8 characters")
	ErrInvalidMerchantRole    = errors.New("invalid merchant user role")
	ErrDashboardSession       = errors.New("invalid dashboard session")
	ErrBootstrapAlreadyExists = errors.New("merchant already has dashboard users")
)

type MerchantUser struct {
	ID           string
	MerchantID   string
	Email        string
	PasswordHash string
	Role         MerchantUserRole
	Status       MerchantUserStatus
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (u MerchantUser) ValidateForBootstrap(password string) error {
	if strings.TrimSpace(strings.ToLower(u.Email)) == "" {
		return ErrInvalidMerchantUser
	}
	if len(strings.TrimSpace(password)) < 8 {
		return ErrInvalidMerchantPass
	}
	if err := ValidateMerchantUserRole(u.Role); err != nil {
		return err
	}
	return nil
}

func ValidateMerchantUserRole(role MerchantUserRole) error {
	switch role {
	case MerchantUserRoleAdmin, MerchantUserRoleDeveloper, MerchantUserRoleReadonly, MerchantUserRoleOps:
		return nil
	default:
		return ErrInvalidMerchantRole
	}
}

func ScopeForMerchantUserRole(role MerchantUserRole) APIKeyScope {
	switch role {
	case MerchantUserRoleAdmin, MerchantUserRoleOps:
		return APIKeyScopeAdmin
	case MerchantUserRoleDeveloper:
		return APIKeyScopeWrite
	default:
		return APIKeyScopeRead
	}
}
