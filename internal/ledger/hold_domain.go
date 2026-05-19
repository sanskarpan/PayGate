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
