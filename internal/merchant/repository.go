package merchant

import (
	"context"
	"time"
)

type Repository interface {
	CreateMerchant(ctx context.Context, merchant Merchant) (Merchant, error)
	GetMerchantByID(ctx context.Context, merchantID string) (Merchant, error)
	GetOnboardingApplicationByMerchant(ctx context.Context, merchantID string) (OnboardingApplication, error)
	UpsertOnboardingApplication(ctx context.Context, app OnboardingApplication, actor, actorScope string) (OnboardingApplication, error)
	TransitionOnboardingApplication(ctx context.Context, merchantID string, nextState OnboardingState, reviewerNotes, actor, actorScope string) (OnboardingApplication, error)
	ListOnboardingParties(ctx context.Context, merchantID string) ([]OnboardingParty, error)
	ReplaceOnboardingParties(ctx context.Context, merchantID string, parties []OnboardingParty, actor, actorScope string) ([]OnboardingParty, error)
	ListOnboardingDocuments(ctx context.Context, merchantID string) ([]OnboardingDocument, error)
	RequestOnboardingDocument(ctx context.Context, merchantID string, doc OnboardingDocument, actor, actorScope string) (OnboardingDocument, error)
	UploadOnboardingDocument(ctx context.Context, merchantID string, doc OnboardingDocument, actor, actorScope string) (OnboardingDocument, error)
	ReviewOnboardingDocument(ctx context.Context, merchantID, documentID string, status DocumentStatus, reviewNotes string, expiresAt *time.Time, actor, actorScope string) (OnboardingDocument, error)
	ListScreeningCases(ctx context.Context, merchantID string) ([]ScreeningCase, error)
	CreateScreeningCase(ctx context.Context, merchantID string, screening ScreeningCase, actor, actorScope string) (ScreeningCase, error)
	ListCapabilities(ctx context.Context, merchantID string) ([]MerchantCapability, error)
	UpsertCapabilities(ctx context.Context, merchantID string, capabilities []MerchantCapability, actor string) ([]MerchantCapability, error)
	GetReservePolicy(ctx context.Context, merchantID string) (ReservePolicy, error)
	UpsertReservePolicy(ctx context.Context, policy ReservePolicy, actor string) (ReservePolicy, error)
	CreateReserveEscalation(ctx context.Context, escalation ReserveEscalation) (ReserveEscalation, error)
	ListReserveEscalations(ctx context.Context, merchantID string, status ReserveEscalationStatus) ([]ReserveEscalation, error)
	ReviewReserveEscalation(ctx context.Context, merchantID, escalationID string, decision ReserveEscalationStatus, notes, actor string, policyOverride *ReservePolicy) (ReserveEscalation, error)
	CreateAPIKey(ctx context.Context, key APIKey) (APIKey, error)
	GetAPIKeyByID(ctx context.Context, keyID string) (APIKey, error)
	ListAPIKeysByMerchant(ctx context.Context, merchantID string) ([]APIKey, error)
	CountActiveAPIKeysByMerchant(ctx context.Context, merchantID string) (int, error)
	UpdateAPIKeyLastUsed(ctx context.Context, keyID string) error
	RevokeAPIKey(ctx context.Context, merchantID, keyID string) error
	CreateMerchantUser(ctx context.Context, user MerchantUser) (MerchantUser, error)
	GetMerchantUserByID(ctx context.Context, userID string) (MerchantUser, error)
	GetMerchantUserByMerchantAndEmail(ctx context.Context, merchantID, email string) (MerchantUser, error)
	CountMerchantUsersByMerchant(ctx context.Context, merchantID string) (int, error)
	UpdateMerchantUserLastLogin(ctx context.Context, userID string) error

	// Team invitations
	CreateInvitation(ctx context.Context, inv Invitation) (Invitation, error)
	GetInvitationByTokenHash(ctx context.Context, tokenHash string) (Invitation, error)
	ListInvitationsByMerchant(ctx context.Context, merchantID string) ([]Invitation, error)
	MarkInvitationAccepted(ctx context.Context, invitationID string) error
	RevokeInvitation(ctx context.Context, merchantID, invitationID string) error

	// API key IP allowlist
	UpdateAPIKeyAllowedIPs(ctx context.Context, merchantID, keyID string, ips []string) error
}
