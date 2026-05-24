package merchant

import (
	"context"
	"strings"
	"time"
)

type UpsertOnboardingPartyInput struct {
	PartyType          OnboardingPartyType `json:"party_type"`
	FullName           string              `json:"full_name"`
	Title              string              `json:"title"`
	Email              string              `json:"email"`
	Phone              string              `json:"phone"`
	OwnershipBPS       int                 `json:"ownership_bps"`
	VerificationStatus VerificationStatus  `json:"verification_status"`
	EvidenceNotes      string              `json:"evidence_notes"`
}

type RequestDocumentInput struct {
	DocumentType  string `json:"document_type"`
	RequestReason string `json:"request_reason"`
}

type UploadDocumentInput struct {
	DocumentID   string `json:"document_id"`
	DocumentType string `json:"document_type"`
	FileName     string `json:"file_name"`
	ContentType  string `json:"content_type"`
	StorageKey   string `json:"storage_key"`
}

type RunScreeningInput struct {
	ScreeningType string `json:"screening_type"`
	ForceResult   string `json:"force_result"`
}

type CapabilityUpdateInput struct {
	CapabilityCode CapabilityCode   `json:"capability_code"`
	Status         CapabilityStatus `json:"status"`
	Reason         string           `json:"reason"`
}

type UpsertReservePolicyInput struct {
	PolicyType      ReservePolicyType `json:"policy_type"`
	PercentageBPS   int               `json:"percentage_bps"`
	HoldDays        int               `json:"hold_days"`
	ThresholdAmount int64             `json:"threshold_amount"`
	Notes           string            `json:"notes"`
}

type QueueReserveEscalationInput struct {
	MerchantID       string
	RiskEventID      string
	TriggerScore     int
	TriggeredRules   []string
	SuggestedBaseBPS int
	Rationale        string
}

func (s *Service) ListOnboardingParties(ctx context.Context, merchantID string) ([]OnboardingParty, error) {
	return s.repo.ListOnboardingParties(ctx, merchantID)
}

func (s *Service) ReplaceOnboardingParties(ctx context.Context, merchantID string, in []UpsertOnboardingPartyInput, actor, actorScope string) ([]OnboardingParty, error) {
	parties := make([]OnboardingParty, 0, len(in))
	for _, item := range in {
		party := OnboardingParty{
			MerchantID:         merchantID,
			PartyType:          item.PartyType,
			FullName:           strings.TrimSpace(item.FullName),
			Title:              strings.TrimSpace(item.Title),
			Email:              strings.TrimSpace(strings.ToLower(item.Email)),
			Phone:              strings.TrimSpace(item.Phone),
			OwnershipBPS:       item.OwnershipBPS,
			VerificationStatus: item.VerificationStatus,
			EvidenceNotes:      strings.TrimSpace(item.EvidenceNotes),
		}
		if party.VerificationStatus == "" {
			party.VerificationStatus = VerificationStatusPending
		}
		if err := party.Validate(); err != nil {
			return nil, err
		}
		parties = append(parties, party)
	}
	return s.repo.ReplaceOnboardingParties(ctx, merchantID, parties, actor, actorScope)
}

func (s *Service) ListOnboardingDocuments(ctx context.Context, merchantID string) ([]OnboardingDocument, error) {
	return s.repo.ListOnboardingDocuments(ctx, merchantID)
}

func (s *Service) RequestOnboardingDocument(ctx context.Context, merchantID string, in RequestDocumentInput, actor, actorScope string) (OnboardingDocument, error) {
	doc := OnboardingDocument{
		MerchantID:    merchantID,
		DocumentType:  strings.TrimSpace(in.DocumentType),
		RequestReason: strings.TrimSpace(in.RequestReason),
	}
	return s.repo.RequestOnboardingDocument(ctx, merchantID, doc, actor, actorScope)
}

func (s *Service) UploadOnboardingDocument(ctx context.Context, merchantID string, in UploadDocumentInput, actor, actorScope string) (OnboardingDocument, error) {
	doc := OnboardingDocument{
		ID:           strings.TrimSpace(in.DocumentID),
		MerchantID:   merchantID,
		DocumentType: strings.TrimSpace(in.DocumentType),
		FileName:     strings.TrimSpace(in.FileName),
		ContentType:  strings.TrimSpace(in.ContentType),
		StorageKey:   strings.TrimSpace(in.StorageKey),
	}
	return s.repo.UploadOnboardingDocument(ctx, merchantID, doc, actor, actorScope)
}

func (s *Service) ReviewOnboardingDocument(ctx context.Context, merchantID, documentID string, status DocumentStatus, reviewNotes string, expiresAt *time.Time, actor, actorScope string) (OnboardingDocument, error) {
	switch status {
	case DocumentStatusApproved, DocumentStatusRejected, DocumentStatusExpired:
	default:
		return OnboardingDocument{}, ErrInvalidDocumentState
	}
	return s.repo.ReviewOnboardingDocument(ctx, merchantID, documentID, status, strings.TrimSpace(reviewNotes), expiresAt, actor, actorScope)
}

func (s *Service) ListScreeningCases(ctx context.Context, merchantID string) ([]ScreeningCase, error) {
	return s.repo.ListScreeningCases(ctx, merchantID)
}

func (s *Service) RunScreening(ctx context.Context, merchantID string, in RunScreeningInput, actor, actorScope string) (ScreeningCase, error) {
	app, err := s.repo.GetOnboardingApplicationByMerchant(ctx, merchantID)
	if err != nil {
		return ScreeningCase{}, err
	}
	force := strings.ToLower(strings.TrimSpace(in.ForceResult))
	status := ScreeningStatusPassed
	subjectName := app.LegalName
	if subjectName == "" {
		subjectName = merchantID
	}
	loweredSubject := strings.ToLower(subjectName)
	if force == "review" || strings.Contains(loweredSubject, "watchlist") || strings.Contains(loweredSubject, "sanction") {
		status = ScreeningStatusReview
	}
	if force == "failed" {
		status = ScreeningStatusFailed
	}
	if force == "passed" {
		status = ScreeningStatusPassed
	}
	return s.repo.CreateScreeningCase(ctx, merchantID, ScreeningCase{
		ScreeningType:     defaultString(strings.TrimSpace(in.ScreeningType), "kyb"),
		Provider:          "simulated",
		ProviderReference: "scr-" + merchantID,
		SubjectName:       subjectName,
		Status:            status,
		ResultPayload: map[string]any{
			"force_result": in.ForceResult,
			"subject_name": subjectName,
		},
		ReviewedBy: actor,
		ScreenedAt: time.Now().UTC(),
	}, actor, actorScope)
}

func (s *Service) ListCapabilities(ctx context.Context, merchantID string) ([]MerchantCapability, error) {
	return s.repo.ListCapabilities(ctx, merchantID)
}

func (s *Service) UpdateCapabilities(ctx context.Context, merchantID string, updates []CapabilityUpdateInput, actor string) ([]MerchantCapability, error) {
	items := make([]MerchantCapability, 0, len(updates))
	for _, update := range updates {
		if err := ValidateCapabilityCode(update.CapabilityCode); err != nil {
			return nil, err
		}
		items = append(items, MerchantCapability{
			MerchantID:     merchantID,
			CapabilityCode: update.CapabilityCode,
			Status:         update.Status,
			Reason:         strings.TrimSpace(update.Reason),
			UpdatedBy:      actor,
		})
	}
	return s.repo.UpsertCapabilities(ctx, merchantID, items, actor)
}

func (s *Service) CheckCapability(ctx context.Context, merchantID string, capability CapabilityCode) error {
	if err := ValidateCapabilityCode(capability); err != nil {
		return err
	}
	items, err := s.repo.ListCapabilities(ctx, merchantID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.CapabilityCode == capability {
			if item.Status == CapabilityStatusEnabled {
				return nil
			}
			return ErrCapabilityRestricted
		}
	}
	return nil
}

func (s *Service) GetReservePolicy(ctx context.Context, merchantID string) (ReservePolicy, error) {
	return s.repo.GetReservePolicy(ctx, merchantID)
}

func (s *Service) UpsertReservePolicy(ctx context.Context, merchantID string, in UpsertReservePolicyInput, actor string) (ReservePolicy, error) {
	policy := ReservePolicy{
		MerchantID:      merchantID,
		PolicyType:      in.PolicyType,
		PercentageBPS:   in.PercentageBPS,
		HoldDays:        in.HoldDays,
		ThresholdAmount: in.ThresholdAmount,
		Notes:           strings.TrimSpace(in.Notes),
	}
	if err := policy.Validate(); err != nil {
		return ReservePolicy{}, err
	}
	return s.repo.UpsertReservePolicy(ctx, policy, actor)
}

func (s *Service) ListReserveEscalations(ctx context.Context, merchantID string, status ReserveEscalationStatus) ([]ReserveEscalation, error) {
	return s.repo.ListReserveEscalations(ctx, merchantID, status)
}

func (s *Service) QueueReserveEscalation(ctx context.Context, in QueueReserveEscalationInput) (ReserveEscalation, error) {
	current, err := s.repo.GetReservePolicy(ctx, in.MerchantID)
	if err != nil {
		return ReserveEscalation{}, err
	}
	percentage := current.PercentageBPS
	if percentage < in.SuggestedBaseBPS {
		percentage = in.SuggestedBaseBPS
	}
	if percentage == 0 {
		percentage = 1000
	}
	holdDays := current.HoldDays
	if holdDays < 14 {
		holdDays = 14
	}
	threshold := current.ThresholdAmount
	return s.repo.CreateReserveEscalation(ctx, ReserveEscalation{
		MerchantID:               in.MerchantID,
		RiskEventID:              in.RiskEventID,
		TriggerScore:             in.TriggerScore,
		TriggeredRules:           in.TriggeredRules,
		Status:                   ReserveEscalationPending,
		SuggestedPolicyType:      ReservePolicyRollingPercentage,
		SuggestedPercentageBPS:   percentage,
		SuggestedHoldDays:        holdDays,
		SuggestedThresholdAmount: threshold,
		Rationale:                strings.TrimSpace(in.Rationale),
	})
}

func (s *Service) ReviewReserveEscalation(ctx context.Context, merchantID, escalationID string, decision ReserveEscalationStatus, notes, actor string, override *UpsertReservePolicyInput) (ReserveEscalation, error) {
	var policyOverride *ReservePolicy
	if override != nil {
		policy := ReservePolicy{
			MerchantID:      merchantID,
			PolicyType:      override.PolicyType,
			PercentageBPS:   override.PercentageBPS,
			HoldDays:        override.HoldDays,
			ThresholdAmount: override.ThresholdAmount,
			Notes:           strings.TrimSpace(override.Notes),
		}
		if err := policy.Validate(); err != nil {
			return ReserveEscalation{}, err
		}
		policyOverride = &policy
	}
	return s.repo.ReviewReserveEscalation(ctx, merchantID, escalationID, decision, strings.TrimSpace(notes), actor, policyOverride)
}

func (s *Service) ensureOnboardingReviewReadiness(ctx context.Context, merchantID string) error {
	parties, err := s.repo.ListOnboardingParties(ctx, merchantID)
	if err != nil {
		return err
	}
	var ownersVerified, controllersVerified bool
	for _, party := range parties {
		if party.VerificationStatus != VerificationStatusVerified {
			continue
		}
		switch party.PartyType {
		case PartyTypeBeneficialOwner:
			ownersVerified = true
		case PartyTypeController:
			controllersVerified = true
		}
	}
	if !ownersVerified || !controllersVerified {
		return ErrOnboardingOwnersIncomplete
	}

	documents, err := s.repo.ListOnboardingDocuments(ctx, merchantID)
	if err != nil {
		return err
	}
	if len(documents) == 0 {
		return ErrOnboardingDocumentsIncomplete
	}
	now := time.Now().UTC()
	hasUsableDocument := false
	for _, doc := range documents {
		if doc.IsUsable(now) {
			hasUsableDocument = true
			break
		}
	}
	if !hasUsableDocument {
		return ErrOnboardingDocumentsIncomplete
	}

	screenings, err := s.repo.ListScreeningCases(ctx, merchantID)
	if err != nil {
		return err
	}
	if len(screenings) == 0 || screenings[0].Status != ScreeningStatusPassed {
		return ErrOnboardingScreeningIncomplete
	}
	return nil
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
