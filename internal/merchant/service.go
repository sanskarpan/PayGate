package merchant

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrMerchantNotFound   = errors.New("merchant not found")
	ErrAPIKeyNotFound     = errors.New("api key not found")
	ErrInvalidCredentials = errors.New("invalid api key credentials")
	ErrScopeNotAllowed    = errors.New("api key scope does not permit this action")
)

type Service struct {
	repo    Repository
	session *SessionManager
}

type CreateMerchantInput struct {
	Name         string         `json:"name"`
	Email        string         `json:"email"`
	BusinessType string         `json:"business_type"`
	Settings     map[string]any `json:"settings"`
}

type BootstrapMerchantUserInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type CreateAPIKeyInput struct {
	Mode  APIKeyMode  `json:"mode"`
	Scope APIKeyScope `json:"scope"`
}

type GeneratedAPIKey struct {
	KeyID     string      `json:"key_id"`
	KeySecret string      `json:"key_secret"`
	Mode      APIKeyMode  `json:"mode"`
	Scope     APIKeyScope `json:"scope"`
}

type Option func(*Service)

func WithSessionSecret(secret string) Option {
	return func(s *Service) {
		s.session = NewSessionManager(secret, time.Hour)
	}
}

func NewService(repo Repository, opts ...Option) *Service {
	svc := &Service{repo: repo}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func (s *Service) CreateMerchant(ctx context.Context, in CreateMerchantInput) (Merchant, error) {
	m := Merchant{
		ID:           idgen.New("merch"),
		Name:         strings.TrimSpace(in.Name),
		Email:        strings.TrimSpace(strings.ToLower(in.Email)),
		BusinessType: strings.TrimSpace(in.BusinessType),
		Status:       MerchantStatusActive,
		Settings:     in.Settings,
	}

	if err := m.ValidateForCreate(); err != nil {
		return Merchant{}, err
	}

	return s.repo.CreateMerchant(ctx, m)
}

func (s *Service) CreateAPIKey(ctx context.Context, merchantID string, in CreateAPIKeyInput) (GeneratedAPIKey, error) {
	if err := ValidateMode(in.Mode); err != nil {
		return GeneratedAPIKey{}, err
	}
	if err := ValidateScope(in.Scope); err != nil {
		return GeneratedAPIKey{}, err
	}

	m, err := s.repo.GetMerchantByID(ctx, merchantID)
	if err != nil {
		return GeneratedAPIKey{}, err
	}
	if err := m.CanIssueAPIKey(); err != nil {
		return GeneratedAPIKey{}, err
	}

	secret, err := generateSecret()
	if err != nil {
		return GeneratedAPIKey{}, fmt.Errorf("generate api key secret: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return GeneratedAPIKey{}, fmt.Errorf("hash api key secret: %w", err)
	}

	prefix := "rzp_test"
	if in.Mode == APIKeyModeLive {
		prefix = "rzp_live"
	}

	key := APIKey{
		ID:         idgen.New(prefix),
		MerchantID: merchantID,
		SecretHash: string(hash),
		Mode:       in.Mode,
		Scope:      in.Scope,
		Status:     APIKeyStatusActive,
	}
	persisted, err := s.repo.CreateAPIKey(ctx, key)
	if err != nil {
		return GeneratedAPIKey{}, err
	}

	return GeneratedAPIKey{
		KeyID:     persisted.ID,
		KeySecret: secret,
		Mode:      persisted.Mode,
		Scope:     persisted.Scope,
	}, nil
}

func (s *Service) RevokeAPIKey(ctx context.Context, merchantID, keyID string) error {
	if strings.TrimSpace(merchantID) == "" || strings.TrimSpace(keyID) == "" {
		return ErrAPIKeyNotFound
	}
	return s.repo.RevokeAPIKey(ctx, merchantID, keyID)
}

func (s *Service) AuthenticateAPIKey(ctx context.Context, keyID, keySecret string, requiredScope APIKeyScope) (APIKey, error) {
	key, err := s.repo.GetAPIKeyByID(ctx, keyID)
	if err != nil {
		return APIKey{}, err
	}
	if key.Status != APIKeyStatusActive {
		return APIKey{}, ErrAPIKeyNotActive
	}
	if bcrypt.CompareHashAndPassword([]byte(key.SecretHash), []byte(keySecret)) != nil {
		return APIKey{}, ErrInvalidCredentials
	}
	if !ScopeAllows(key.Scope, requiredScope) {
		return APIKey{}, ErrScopeNotAllowed
	}
	_ = s.repo.UpdateAPIKeyLastUsed(ctx, key.ID)
	return key, nil
}

func (s *Service) CanBootstrapAPIKey(ctx context.Context, merchantID string) (bool, error) {
	count, err := s.repo.CountActiveAPIKeysByMerchant(ctx, merchantID)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, merchantID string) ([]APIKey, error) {
	return s.repo.ListAPIKeysByMerchant(ctx, merchantID)
}

func (s *Service) BootstrapMerchantUser(ctx context.Context, merchantID string, in BootstrapMerchantUserInput) (MerchantUser, error) {
	user := MerchantUser{
		ID:         idgen.New("muser"),
		MerchantID: strings.TrimSpace(merchantID),
		Email:      strings.TrimSpace(strings.ToLower(in.Email)),
		Role:       MerchantUserRoleAdmin,
		Status:     MerchantUserStatusActive,
	}
	if err := user.ValidateForBootstrap(in.Password); err != nil {
		return MerchantUser{}, err
	}
	count, err := s.repo.CountMerchantUsersByMerchant(ctx, user.MerchantID)
	if err != nil {
		return MerchantUser{}, err
	}
	if count > 0 {
		return MerchantUser{}, ErrBootstrapAlreadyExists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(strings.TrimSpace(in.Password)), bcrypt.DefaultCost)
	if err != nil {
		return MerchantUser{}, fmt.Errorf("hash merchant user password: %w", err)
	}
	user.PasswordHash = string(hash)
	return s.repo.CreateMerchantUser(ctx, user)
}

func (s *Service) AuthenticateMerchantUser(ctx context.Context, merchantID, email, password string) (MerchantUser, error) {
	user, err := s.repo.GetMerchantUserByMerchantAndEmail(ctx, strings.TrimSpace(merchantID), strings.TrimSpace(strings.ToLower(email)))
	if err != nil {
		return MerchantUser{}, err
	}
	if user.Status != MerchantUserStatusActive {
		return MerchantUser{}, ErrMerchantUserNotActive
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return MerchantUser{}, ErrInvalidCredentials
	}
	_ = s.repo.UpdateMerchantUserLastLogin(ctx, user.ID)
	return user, nil
}

func (s *Service) IssueDashboardSession(user MerchantUser) (string, error) {
	if s.session == nil {
		return "", ErrDashboardSession
	}
	return s.session.Issue(user)
}

func (s *Service) DashboardSessionTTL() time.Duration {
	if s.session == nil {
		return time.Hour
	}
	return s.session.TTL()
}

func (s *Service) AuthenticateDashboardSession(ctx context.Context, token string, requiredScope APIKeyScope) (MerchantUser, error) {
	if s.session == nil {
		return MerchantUser{}, ErrDashboardSession
	}
	claims, err := s.session.Parse(token)
	if err != nil {
		return MerchantUser{}, err
	}
	user, err := s.repo.GetMerchantUserByID(ctx, claims.UserID)
	if err != nil {
		return MerchantUser{}, err
	}
	if user.Status != MerchantUserStatusActive {
		return MerchantUser{}, ErrMerchantUserNotActive
	}
	if user.MerchantID != claims.MerchantID || user.Email != claims.Email || user.Role != claims.Role {
		return MerchantUser{}, ErrDashboardSession
	}
	if !ScopeAllows(ScopeForMerchantUserRole(user.Role), requiredScope) {
		return MerchantUser{}, ErrScopeNotAllowed
	}
	return user, nil
}

func generateSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "pgs_" + base64.RawURLEncoding.EncodeToString(raw), nil
}
