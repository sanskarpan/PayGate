package merchant

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"golang.org/x/crypto/bcrypt"
)

// InvitationStatus tracks the lifecycle of a team invitation.
type InvitationStatus string

const (
	InvitationStatusPending  InvitationStatus = "pending"
	InvitationStatusAccepted InvitationStatus = "accepted"
	InvitationStatusExpired  InvitationStatus = "expired"
	InvitationStatusRevoked  InvitationStatus = "revoked"

	// InvitationTTL is how long an invitation token remains valid.
	InvitationTTL = 72 * time.Hour
)

var (
	ErrInvitationNotFound = errors.New("invitation not found")
	ErrInvitationExpired  = errors.New("invitation has expired")
	ErrInvitationInvalid  = errors.New("invalid invitation token")
	ErrInvitationUsed     = errors.New("invitation has already been accepted or revoked")
)

// Invitation represents a pending team membership invitation.
type Invitation struct {
	ID         string
	MerchantID string
	Email      string
	Role       MerchantUserRole
	TokenHash  string
	Status     InvitationStatus
	InvitedBy  string // user ID of the inviter
	ExpiresAt  time.Time
	AcceptedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// InviteUserInput holds the parameters for creating an invitation.
type InviteUserInput struct {
	Email     string           `json:"email"`
	Role      MerchantUserRole `json:"role"`
	InvitedBy string           // set from the authenticated principal
}

// AcceptInvitationInput holds the parameters needed to accept an invitation.
type AcceptInvitationInput struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// InviteUser creates an invitation token and persists the invitation record.
// It returns the plain-text token (shown once; not stored).
func (s *Service) InviteUser(ctx context.Context, merchantID string, in InviteUserInput) (Invitation, string, error) {
	if err := ValidateMerchantUserRole(in.Role); err != nil {
		return Invitation{}, "", err
	}
	email := strings.TrimSpace(strings.ToLower(in.Email))
	if email == "" {
		return Invitation{}, "", ErrInvalidMerchantUser
	}

	// Generate a 32-byte random token.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Invitation{}, "", err
	}
	token := hex.EncodeToString(raw)

	// Store SHA-256 hash of the token (not bcrypt — lookups must be fast).
	sum := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(sum[:])

	inv := Invitation{
		ID:         idgen.New("inv"),
		MerchantID: merchantID,
		Email:      email,
		Role:       in.Role,
		TokenHash:  tokenHash,
		Status:     InvitationStatusPending,
		InvitedBy:  in.InvitedBy,
		ExpiresAt:  time.Now().Add(InvitationTTL),
	}

	created, err := s.repo.CreateInvitation(ctx, inv)
	if err != nil {
		return Invitation{}, "", err
	}
	return created, token, nil
}

// AcceptInvitation validates the token, creates a new MerchantUser, and marks
// the invitation as accepted — all in a single service call.
func (s *Service) AcceptInvitation(ctx context.Context, in AcceptInvitationInput) (MerchantUser, error) {
	sum := sha256.Sum256([]byte(in.Token))
	tokenHash := hex.EncodeToString(sum[:])

	inv, err := s.repo.GetInvitationByTokenHash(ctx, tokenHash)
	if err != nil {
		return MerchantUser{}, ErrInvitationInvalid
	}
	if inv.Status != InvitationStatusPending {
		return MerchantUser{}, ErrInvitationUsed
	}
	if time.Now().After(inv.ExpiresAt) {
		return MerchantUser{}, ErrInvitationExpired
	}

	trimmed := strings.TrimSpace(in.Password)
	if len(trimmed) < 8 || len(trimmed) > 72 {
		return MerchantUser{}, ErrInvalidMerchantPass
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(trimmed), bcrypt.DefaultCost)
	if err != nil {
		return MerchantUser{}, err
	}

	user := MerchantUser{
		ID:           idgen.New("muser"),
		MerchantID:   inv.MerchantID,
		Email:        inv.Email,
		PasswordHash: string(hash),
		Role:         inv.Role,
		Status:       MerchantUserStatusActive,
	}
	created, err := s.repo.CreateMerchantUser(ctx, user)
	if err != nil {
		return MerchantUser{}, err
	}

	if err := s.repo.MarkInvitationAccepted(ctx, inv.ID); err != nil {
		return MerchantUser{}, err
	}
	return created, nil
}

// ListInvitations returns all invitations for a merchant.
func (s *Service) ListInvitations(ctx context.Context, merchantID string) ([]Invitation, error) {
	return s.repo.ListInvitationsByMerchant(ctx, merchantID)
}

// RevokeInvitation cancels a pending invitation.
func (s *Service) RevokeInvitation(ctx context.Context, merchantID, invitationID string) error {
	return s.repo.RevokeInvitation(ctx, merchantID, invitationID)
}
