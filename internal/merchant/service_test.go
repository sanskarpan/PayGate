package merchant

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRepo struct {
	merchants map[string]Merchant
	keys      map[string]APIKey
	users     map[string]MerchantUser
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{merchants: map[string]Merchant{}, keys: map[string]APIKey{}, users: map[string]MerchantUser{}}
}

func (f *fakeRepo) CreateMerchant(_ context.Context, m Merchant) (Merchant, error) {
	f.merchants[m.ID] = m
	return m, nil
}

func (f *fakeRepo) GetMerchantByID(_ context.Context, id string) (Merchant, error) {
	m, ok := f.merchants[id]
	if !ok {
		return Merchant{}, ErrMerchantNotFound
	}
	return m, nil
}

func (f *fakeRepo) CreateAPIKey(_ context.Context, k APIKey) (APIKey, error) {
	f.keys[k.ID] = k
	return k, nil
}

func (f *fakeRepo) GetAPIKeyByID(_ context.Context, id string) (APIKey, error) {
	k, ok := f.keys[id]
	if !ok {
		return APIKey{}, ErrAPIKeyNotFound
	}
	return k, nil
}

func (f *fakeRepo) ListAPIKeysByMerchant(_ context.Context, merchantID string) ([]APIKey, error) {
	keys := make([]APIKey, 0)
	for _, key := range f.keys {
		if key.MerchantID == merchantID {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (f *fakeRepo) CountActiveAPIKeysByMerchant(_ context.Context, merchantID string) (int, error) {
	count := 0
	for _, k := range f.keys {
		if k.MerchantID == merchantID && k.Status == APIKeyStatusActive {
			count++
		}
	}
	return count, nil
}

func (f *fakeRepo) UpdateAPIKeyLastUsed(_ context.Context, _ string) error { return nil }

func (f *fakeRepo) RevokeAPIKey(_ context.Context, merchantID, keyID string) error {
	k, ok := f.keys[keyID]
	if !ok || k.MerchantID != merchantID {
		return ErrAPIKeyNotFound
	}
	k.Status = APIKeyStatusRevoked
	f.keys[keyID] = k
	return nil
}

func (f *fakeRepo) CreateMerchantUser(_ context.Context, user MerchantUser) (MerchantUser, error) {
	f.users[user.ID] = user
	return user, nil
}

func (f *fakeRepo) GetMerchantUserByID(_ context.Context, userID string) (MerchantUser, error) {
	user, ok := f.users[userID]
	if !ok {
		return MerchantUser{}, ErrMerchantUserNotFound
	}
	return user, nil
}

func (f *fakeRepo) GetMerchantUserByMerchantAndEmail(_ context.Context, merchantID, email string) (MerchantUser, error) {
	for _, user := range f.users {
		if user.MerchantID == merchantID && user.Email == email {
			return user, nil
		}
	}
	return MerchantUser{}, ErrMerchantUserNotFound
}

func (f *fakeRepo) CountMerchantUsersByMerchant(_ context.Context, merchantID string) (int, error) {
	count := 0
	for _, user := range f.users {
		if user.MerchantID == merchantID {
			count++
		}
	}
	return count, nil
}

func (f *fakeRepo) UpdateMerchantUserLastLogin(_ context.Context, _ string) error {
	return nil
}

// Invitation stubs — not exercised by unit tests but required by interface.
func (f *fakeRepo) CreateInvitation(_ context.Context, inv Invitation) (Invitation, error) {
	return inv, nil
}
func (f *fakeRepo) GetInvitationByTokenHash(_ context.Context, _ string) (Invitation, error) {
	return Invitation{}, ErrInvitationNotFound
}
func (f *fakeRepo) ListInvitationsByMerchant(_ context.Context, _ string) ([]Invitation, error) {
	return nil, nil
}
func (f *fakeRepo) MarkInvitationAccepted(_ context.Context, _ string) error { return nil }
func (f *fakeRepo) RevokeInvitation(_ context.Context, _, _ string) error    { return nil }
func (f *fakeRepo) UpdateAPIKeyAllowedIPs(_ context.Context, _, _ string, _ []string) error {
	return nil
}

func TestCreateAPIKey(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	m, err := svc.CreateMerchant(context.Background(), CreateMerchantInput{
		Name:         "ACME",
		Email:        "ops@acme.com",
		BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}

	out, err := svc.CreateAPIKey(context.Background(), m.ID, CreateAPIKeyInput{Mode: APIKeyModeTest, Scope: APIKeyScopeWrite})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	if !strings.HasPrefix(out.KeyID, "rzp_test_") {
		t.Fatalf("expected rzp_test_ prefix, got %s", out.KeyID)
	}
	if out.KeySecret == "" {
		t.Fatal("expected key secret")
	}
}

func TestAuthenticateAPIKeyInvalidSecret(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	m := Merchant{ID: "merch_test", Name: "ACME", Email: "ops@acme.com", BusinessType: "company", Status: MerchantStatusActive}
	repo.merchants[m.ID] = m

	created, err := svc.CreateAPIKey(context.Background(), m.ID, CreateAPIKeyInput{Mode: APIKeyModeTest, Scope: APIKeyScopeRead})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	_, err = svc.AuthenticateAPIKey(context.Background(), created.KeyID, "wrong", APIKeyScopeRead)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}
