package merchant

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeRepo struct {
	merchants     map[string]Merchant
	keys          map[string]APIKey
	users         map[string]MerchantUser
	onboarding    map[string]OnboardingApplication
	parties       map[string][]OnboardingParty
	documents     map[string][]OnboardingDocument
	screenings    map[string][]ScreeningCase
	capabilities  map[string][]MerchantCapability
	reservePolicy map[string]ReservePolicy
	escalations   map[string][]ReserveEscalation
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		merchants:     map[string]Merchant{},
		keys:          map[string]APIKey{},
		users:         map[string]MerchantUser{},
		onboarding:    map[string]OnboardingApplication{},
		parties:       map[string][]OnboardingParty{},
		documents:     map[string][]OnboardingDocument{},
		screenings:    map[string][]ScreeningCase{},
		capabilities:  map[string][]MerchantCapability{},
		reservePolicy: map[string]ReservePolicy{},
		escalations:   map[string][]ReserveEscalation{},
	}
}

func (f *fakeRepo) CreateMerchant(_ context.Context, m Merchant) (Merchant, error) {
	f.merchants[m.ID] = m
	f.onboarding[m.ID] = OnboardingApplication{ID: "kyb_" + m.ID, MerchantID: m.ID, CountryCode: "IN", State: OnboardingStateDraft}
	f.capabilities[m.ID] = DefaultCapabilities(m.ID)
	f.reservePolicy[m.ID] = DefaultReservePolicy(m.ID)
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

func (f *fakeRepo) GetOnboardingApplicationByMerchant(_ context.Context, merchantID string) (OnboardingApplication, error) {
	app, ok := f.onboarding[merchantID]
	if !ok {
		return OnboardingApplication{}, ErrOnboardingApplicationNotFound
	}
	return app, nil
}

func (f *fakeRepo) UpsertOnboardingApplication(_ context.Context, app OnboardingApplication, _, _ string) (OnboardingApplication, error) {
	f.onboarding[app.MerchantID] = app
	if m, ok := f.merchants[app.MerchantID]; ok {
		m.OnboardingStatus = app.State
		f.merchants[app.MerchantID] = m
	}
	return app, nil
}

func (f *fakeRepo) TransitionOnboardingApplication(_ context.Context, merchantID string, nextState OnboardingState, reviewerNotes, _, _ string) (OnboardingApplication, error) {
	app, ok := f.onboarding[merchantID]
	if !ok {
		return OnboardingApplication{}, ErrOnboardingApplicationNotFound
	}
	app.State = nextState
	app.ReviewerNotes = reviewerNotes
	f.onboarding[merchantID] = app
	if m, ok := f.merchants[merchantID]; ok {
		m.OnboardingStatus = nextState
		f.merchants[merchantID] = m
	}
	return app, nil
}

func (f *fakeRepo) ListOnboardingParties(_ context.Context, merchantID string) ([]OnboardingParty, error) {
	return append([]OnboardingParty(nil), f.parties[merchantID]...), nil
}

func (f *fakeRepo) ReplaceOnboardingParties(_ context.Context, merchantID string, parties []OnboardingParty, _, _ string) ([]OnboardingParty, error) {
	f.parties[merchantID] = append([]OnboardingParty(nil), parties...)
	return append([]OnboardingParty(nil), parties...), nil
}

func (f *fakeRepo) ListOnboardingDocuments(_ context.Context, merchantID string) ([]OnboardingDocument, error) {
	return append([]OnboardingDocument(nil), f.documents[merchantID]...), nil
}

func (f *fakeRepo) RequestOnboardingDocument(_ context.Context, merchantID string, doc OnboardingDocument, _, _ string) (OnboardingDocument, error) {
	doc.ID = "doc_" + merchantID
	doc.MerchantID = merchantID
	doc.Status = DocumentStatusRequested
	f.documents[merchantID] = append(f.documents[merchantID], doc)
	return doc, nil
}

func (f *fakeRepo) UploadOnboardingDocument(_ context.Context, merchantID string, doc OnboardingDocument, _, _ string) (OnboardingDocument, error) {
	doc.MerchantID = merchantID
	doc.Status = DocumentStatusUploaded
	if doc.ID == "" {
		doc.ID = "doc_upload_" + merchantID
		f.documents[merchantID] = append(f.documents[merchantID], doc)
		return doc, nil
	}
	items := f.documents[merchantID]
	for i := range items {
		if items[i].ID == doc.ID {
			items[i].FileName = doc.FileName
			items[i].ContentType = doc.ContentType
			items[i].StorageKey = doc.StorageKey
			items[i].Status = DocumentStatusUploaded
			doc = items[i]
			f.documents[merchantID] = items
			return doc, nil
		}
	}
	f.documents[merchantID] = append(f.documents[merchantID], doc)
	return doc, nil
}

func (f *fakeRepo) ReviewOnboardingDocument(_ context.Context, merchantID, documentID string, status DocumentStatus, reviewNotes string, expiresAt *time.Time, _, _ string) (OnboardingDocument, error) {
	items := f.documents[merchantID]
	for i := range items {
		if items[i].ID == documentID {
			items[i].Status = status
			items[i].ReviewNotes = reviewNotes
			items[i].ExpiresAt = expiresAt
			f.documents[merchantID] = items
			return items[i], nil
		}
	}
	return OnboardingDocument{}, ErrOnboardingApplicationNotFound
}

func (f *fakeRepo) ListScreeningCases(_ context.Context, merchantID string) ([]ScreeningCase, error) {
	return append([]ScreeningCase(nil), f.screenings[merchantID]...), nil
}

func (f *fakeRepo) CreateScreeningCase(_ context.Context, merchantID string, screening ScreeningCase, _, _ string) (ScreeningCase, error) {
	f.screenings[merchantID] = append([]ScreeningCase{screening}, f.screenings[merchantID]...)
	return screening, nil
}

func (f *fakeRepo) ListCapabilities(_ context.Context, merchantID string) ([]MerchantCapability, error) {
	return append([]MerchantCapability(nil), f.capabilities[merchantID]...), nil
}

func (f *fakeRepo) UpsertCapabilities(_ context.Context, merchantID string, capabilities []MerchantCapability, actor string) ([]MerchantCapability, error) {
	current := f.capabilities[merchantID]
	index := map[CapabilityCode]int{}
	for i := range current {
		index[current[i].CapabilityCode] = i
	}
	for _, capability := range capabilities {
		capability.MerchantID = merchantID
		capability.UpdatedBy = actor
		if pos, ok := index[capability.CapabilityCode]; ok {
			current[pos] = capability
		} else {
			current = append(current, capability)
		}
	}
	f.capabilities[merchantID] = current
	return append([]MerchantCapability(nil), current...), nil
}

func (f *fakeRepo) GetReservePolicy(_ context.Context, merchantID string) (ReservePolicy, error) {
	return f.reservePolicy[merchantID], nil
}

func (f *fakeRepo) UpsertReservePolicy(_ context.Context, policy ReservePolicy, _ string) (ReservePolicy, error) {
	f.reservePolicy[policy.MerchantID] = policy
	return policy, nil
}

func (f *fakeRepo) CreateReserveEscalation(_ context.Context, escalation ReserveEscalation) (ReserveEscalation, error) {
	if escalation.ID == "" {
		escalation.ID = "resc_" + escalation.MerchantID
	}
	f.escalations[escalation.MerchantID] = append([]ReserveEscalation{escalation}, f.escalations[escalation.MerchantID]...)
	return escalation, nil
}

func (f *fakeRepo) ListReserveEscalations(_ context.Context, merchantID string, status ReserveEscalationStatus) ([]ReserveEscalation, error) {
	items := f.escalations[merchantID]
	if status == "" {
		return append([]ReserveEscalation(nil), items...), nil
	}
	var out []ReserveEscalation
	for _, item := range items {
		if item.Status == status {
			out = append(out, item)
		}
	}
	return out, nil
}

func (f *fakeRepo) ReviewReserveEscalation(_ context.Context, merchantID, escalationID string, decision ReserveEscalationStatus, notes, actor string, policyOverride *ReservePolicy) (ReserveEscalation, error) {
	items := f.escalations[merchantID]
	for i := range items {
		if items[i].ID == escalationID {
			items[i].Status = decision
			items[i].ReviewNotes = notes
			items[i].ReviewedBy = actor
			now := time.Now().UTC()
			items[i].ReviewedAt = &now
			f.escalations[merchantID] = items
			if decision == ReserveEscalationApproved {
				policy := ReservePolicy{
					MerchantID:      merchantID,
					PolicyType:      items[i].SuggestedPolicyType,
					PercentageBPS:   items[i].SuggestedPercentageBPS,
					HoldDays:        items[i].SuggestedHoldDays,
					ThresholdAmount: items[i].SuggestedThresholdAmount,
				}
				if policyOverride != nil {
					policy = *policyOverride
				}
				f.reservePolicy[merchantID] = policy
			}
			return items[i], nil
		}
	}
	return ReserveEscalation{}, ErrReserveEscalationNotFound
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

func TestCreateLiveAPIKeyRequiresApprovedOnboarding(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	m, err := svc.CreateMerchant(context.Background(), CreateMerchantInput{
		Name:         "ACME",
		Email:        "live@acme.com",
		BusinessType: "company",
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}

	_, err = svc.CreateAPIKey(context.Background(), m.ID, CreateAPIKeyInput{Mode: APIKeyModeLive, Scope: APIKeyScopeAdmin})
	if !errors.Is(err, ErrOnboardingNotApproved) {
		t.Fatalf("expected ErrOnboardingNotApproved, got %v", err)
	}

	_, err = svc.UpsertOnboardingApplication(context.Background(), m.ID, UpsertOnboardingApplicationInput{
		LegalName:              "ACME Pvt Ltd",
		BusinessClassification: "private_limited",
		RegistrationNumber:     "U12345KA2026PTC100001",
		TaxIdentifier:          "29ABCDE1234F1Z9",
		CountryCode:            "IN",
	}, "test", "admin")
	if err != nil {
		t.Fatalf("upsert onboarding: %v", err)
	}
	if _, err := svc.SubmitOnboardingApplication(context.Background(), m.ID, "test", "admin"); err != nil {
		t.Fatalf("submit onboarding: %v", err)
	}
	if _, err := svc.ReplaceOnboardingParties(context.Background(), m.ID, []UpsertOnboardingPartyInput{
		{PartyType: PartyTypeBeneficialOwner, FullName: "Owner One", OwnershipBPS: 6000, VerificationStatus: VerificationStatusVerified},
		{PartyType: PartyTypeController, FullName: "Controller One", VerificationStatus: VerificationStatusVerified},
	}, "test", "admin"); err != nil {
		t.Fatalf("replace onboarding parties: %v", err)
	}
	doc, err := svc.RequestOnboardingDocument(context.Background(), m.ID, RequestDocumentInput{
		DocumentType:  "certificate_of_incorporation",
		RequestReason: "required",
	}, "test", "admin")
	if err != nil {
		t.Fatalf("request onboarding document: %v", err)
	}
	doc, err = svc.UploadOnboardingDocument(context.Background(), m.ID, UploadDocumentInput{
		DocumentID:   doc.ID,
		DocumentType: doc.DocumentType,
		FileName:     "coi.pdf",
		ContentType:  "application/pdf",
		StorageKey:   "merchant/coi.pdf",
	}, "test", "admin")
	if err != nil {
		t.Fatalf("upload onboarding document: %v", err)
	}
	if _, err := svc.ReviewOnboardingDocument(context.Background(), m.ID, doc.ID, DocumentStatusApproved, "ok", nil, "test", "admin"); err != nil {
		t.Fatalf("review onboarding document: %v", err)
	}
	if _, err := svc.RunScreening(context.Background(), m.ID, RunScreeningInput{ForceResult: "passed"}, "test", "admin"); err != nil {
		t.Fatalf("run screening: %v", err)
	}
	if _, err := svc.ReviewOnboardingApplication(context.Background(), m.ID, OnboardingStateApproved, "approved", "test", "admin"); err != nil {
		t.Fatalf("approve onboarding: %v", err)
	}

	out, err := svc.CreateAPIKey(context.Background(), m.ID, CreateAPIKeyInput{Mode: APIKeyModeLive, Scope: APIKeyScopeAdmin})
	if err != nil {
		t.Fatalf("create live api key after approval: %v", err)
	}
	if !strings.HasPrefix(out.KeyID, "rzp_live_") {
		t.Fatalf("expected live key prefix, got %s", out.KeyID)
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
