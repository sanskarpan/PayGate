package merchant

import "context"

type Repository interface {
	CreateMerchant(ctx context.Context, merchant Merchant) (Merchant, error)
	GetMerchantByID(ctx context.Context, merchantID string) (Merchant, error)
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
