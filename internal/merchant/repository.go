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
}
