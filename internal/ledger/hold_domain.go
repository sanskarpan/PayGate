package ledger

import (
	"errors"
	"time"
)

type HoldStatus string

const (
	HoldStatusActive    HoldStatus = "active"
	HoldStatusReleased  HoldStatus = "released"
	HoldStatusCommitted HoldStatus = "committed"
	HoldStatusExpired   HoldStatus = "expired"
)

var (
	ErrHoldNotFound       = errors.New("ledger hold not found")
	ErrHoldNotActive      = errors.New("ledger hold is not active")
	ErrHoldInsufficient   = errors.New("insufficient payoutable balance")
	ErrHoldAlreadyHandled = errors.New("ledger hold is already finalized")
	// ErrInvalidHoldInput marks a caller mistake so the handler can answer 400
	// instead of falling through to a 500.
	ErrInvalidHoldInput = errors.New("account_code, source_type and source_id are required and amount must be positive")
)

type Hold struct {
	ID                string
	MerchantID        string
	AccountCode       string
	SourceType        string
	SourceID          string
	Reason            string
	Currency          string
	Amount            int64
	Status            HoldStatus
	IdempotencyKey    string
	TargetAccountCode string
	Description       string
	ExpiresAt         *time.Time
	ReleasedAt        *time.Time
	CommittedAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type CreateHoldInput struct {
	MerchantID     string
	AccountCode    string
	SourceType     string
	SourceID       string
	Reason         string
	Currency       string
	Amount         int64
	IdempotencyKey string
	ExpiresAt      *time.Time
}

type ReleaseHoldInput struct {
	MerchantID string
	HoldID     string
}

type ExtendHoldInput struct {
	MerchantID string
	HoldID     string
	ExpiresAt  *time.Time
}

type CommitHoldInput struct {
	MerchantID        string
	HoldID            string
	TargetAccountCode string
	Description       string
}

type ExpireHoldInput struct {
	MerchantID string
	HoldID     string
}
