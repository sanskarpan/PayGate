package upiverify

import (
	"context"
	"testing"
	"time"
)

type memoryRepo struct {
	items []Verification
}

func (m *memoryRepo) GetLatest(_ context.Context, merchantID, vpa string, purpose Purpose) (Verification, error) {
	for i := len(m.items) - 1; i >= 0; i-- {
		item := m.items[i]
		if item.MerchantID == merchantID && item.VPA == vpa && item.Purpose == purpose {
			return item, nil
		}
	}
	return Verification{}, ErrVPAVerificationMiss
}

func (m *memoryRepo) Record(_ context.Context, verification Verification) (Verification, error) {
	verification.Version = len(m.items) + 1
	verification.ID = "vpaver_test"
	verification.CreatedAt = verification.VerifiedAt
	m.items = append(m.items, verification)
	return verification, nil
}

func TestNormalizeVPA(t *testing.T) {
	normalized, err := NormalizeVPA(" Customer.Test@UPI ")
	if err != nil {
		t.Fatalf("normalize vpa: %v", err)
	}
	if normalized != "customer.test@upi" {
		t.Fatalf("expected normalized vpa, got %s", normalized)
	}
	if _, err := NormalizeVPA("bad vpa"); err == nil {
		t.Fatalf("expected invalid vpa error")
	}
}

func TestEnsureFreshReusesVerification(t *testing.T) {
	now := time.Now().UTC()
	repo := &memoryRepo{
		items: []Verification{{
			ID:         "vpaver_existing",
			MerchantID: "m_1",
			VPA:        "customer@upi",
			Purpose:    PurposeMandate,
			Version:    1,
			Status:     StatusVerified,
			VerifiedAt: now.Add(-time.Hour),
			ExpiresAt:  now.Add(24 * time.Hour),
		}},
	}
	svc := NewService(repo, NewSimulatorProvider())
	out, err := svc.EnsureFresh(context.Background(), "m_1", "customer@upi", PurposeMandate, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("ensure fresh: %v", err)
	}
	if out.ID != "vpaver_existing" {
		t.Fatalf("expected existing verification reuse, got %#v", out)
	}
	if len(repo.items) != 1 {
		t.Fatalf("expected no new record, got %d", len(repo.items))
	}
}
