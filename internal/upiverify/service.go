package upiverify

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
)

type Provider interface {
	VerifyVPA(ctx context.Context, merchantID, vpa string, purpose Purpose) (Result, error)
}

type Service struct {
	repo     Repository
	provider Provider
}

func NewService(repo Repository, provider Provider) *Service {
	return &Service{repo: repo, provider: provider}
}

func (s *Service) ValidateSyntax(vpa string) (string, error) {
	return NormalizeVPA(vpa)
}

func (s *Service) GetLatest(ctx context.Context, merchantID, vpa string, purpose Purpose) (Verification, error) {
	normalized, err := NormalizeVPA(vpa)
	if err != nil {
		return Verification{}, err
	}
	return s.repo.GetLatest(ctx, merchantID, normalized, purpose)
}

func (s *Service) EnsureFresh(ctx context.Context, merchantID, vpa string, purpose Purpose, maxAge time.Duration) (Verification, error) {
	normalized, err := NormalizeVPA(vpa)
	if err != nil {
		return Verification{}, err
	}
	if maxAge <= 0 {
		maxAge = 30 * 24 * time.Hour
	}
	now := time.Now().UTC()
	latest, err := s.repo.GetLatest(ctx, merchantID, normalized, purpose)
	if err == nil && latest.Status == StatusVerified && latest.ExpiresAt.After(now) && latest.VerifiedAt.Add(maxAge).After(now) {
		return latest, nil
	}
	if err != nil && !errors.Is(err, ErrVPAVerificationMiss) {
		return Verification{}, err
	}
	if s.provider == nil {
		return Verification{}, ErrVPAVerificationMiss
	}
	result, err := s.provider.VerifyVPA(ctx, merchantID, normalized, purpose)
	if err != nil {
		return Verification{}, err
	}
	if result.VerifiedAt.IsZero() {
		result.VerifiedAt = now
	}
	if result.ExpiresAt.IsZero() {
		result.ExpiresAt = result.VerifiedAt.Add(maxAge)
	}
	if result.Provider == "" {
		result.Provider = "simulated"
	}
	recorded, err := s.repo.Record(ctx, Verification{
		MerchantID:        merchantID,
		VPA:               normalized,
		Purpose:           purpose,
		Status:            result.Status,
		Provider:          result.Provider,
		ProviderReference: result.ProviderReference,
		Evidence:          result.Evidence,
		VerifiedAt:        result.VerifiedAt,
		ExpiresAt:         result.ExpiresAt,
	})
	if err != nil {
		return Verification{}, err
	}
	if recorded.Status != StatusVerified {
		return recorded, ErrInvalidVPA
	}
	return recorded, nil
}

type SimulatorProvider struct{}

func NewSimulatorProvider() *SimulatorProvider { return &SimulatorProvider{} }

func (p *SimulatorProvider) VerifyVPA(_ context.Context, merchantID, vpa string, purpose Purpose) (Result, error) {
	now := time.Now().UTC()
	result := Result{
		Status:            StatusVerified,
		Provider:          "simulated_upi_directory",
		ProviderReference: "upi_verify_" + merchantID + "_" + string(purpose),
		VerifiedAt:        now,
		ExpiresAt:         now.Add(30 * 24 * time.Hour),
		Evidence: map[string]any{
			"vpa":        vpa,
			"purpose":    string(purpose),
			"payee_name": simulatedPayeeName(vpa),
			"psp":        strings.ToUpper(strings.Split(strings.Split(vpa, "@")[1], ".")[0]),
			"verified":   true,
		},
	}
	if strings.Contains(vpa, "invalid") || strings.Contains(vpa, "blocked") || strings.Contains(vpa, "reject") {
		result.Status = StatusRejected
		result.Evidence["verified"] = false
		result.Evidence["reason"] = "directory lookup rejected vpa"
	}
	return result, nil
}

func simulatedPayeeName(vpa string) string {
	localPart := strings.Split(vpa, "@")[0]
	localPart = strings.ReplaceAll(localPart, ".", " ")
	localPart = strings.ReplaceAll(localPart, "-", " ")
	localPart = strings.ReplaceAll(localPart, "_", " ")
	if localPart == "" {
		return "UPI Payee"
	}
	parts := strings.Fields(localPart)
	for i := range parts {
		runes := []rune(parts[i])
		if len(runes) == 0 {
			continue
		}
		runes[0] = unicode.ToUpper(runes[0])
		for j := 1; j < len(runes); j++ {
			runes[j] = unicode.ToLower(runes[j])
		}
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}
