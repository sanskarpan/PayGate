package upiverify

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

type Purpose string
type Status string

const (
	PurposeMandate           Purpose = "mandate"
	PurposePayoutDestination Purpose = "payout_destination"
)

const (
	StatusVerified Status = "verified"
	StatusRejected Status = "rejected"
)

var (
	ErrInvalidVPA          = errors.New("invalid upi vpa")
	ErrVPAVerificationMiss = errors.New("upi vpa verification not found")
)

var vpaPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}@[a-z][a-z0-9.-]{1,63}$`)

type Verification struct {
	ID                string
	MerchantID        string
	VPA               string
	Purpose           Purpose
	Version           int
	Status            Status
	Provider          string
	ProviderReference string
	Evidence          map[string]any
	VerifiedAt        time.Time
	ExpiresAt         time.Time
	CreatedAt         time.Time
}

type Result struct {
	Status            Status
	Provider          string
	ProviderReference string
	Evidence          map[string]any
	VerifiedAt        time.Time
	ExpiresAt         time.Time
}

func NormalizeVPA(vpa string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(vpa))
	if !vpaPattern.MatchString(normalized) {
		return "", ErrInvalidVPA
	}
	return normalized, nil
}
