package merchant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

func (r *PostgresRepository) ListOnboardingParties(ctx context.Context, merchantID string) ([]OnboardingParty, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, application_id, merchant_id, party_type, full_name, title, email, phone, ownership_bps,
       verification_status, evidence_notes, revision, created_at, updated_at
FROM paygate_merchants.merchant_onboarding_parties
WHERE merchant_id = $1 AND superseded_at IS NULL
ORDER BY party_type, created_at, id
`, merchantID)
	if err != nil {
		return nil, fmt.Errorf("list onboarding parties: %w", err)
	}
	defer rows.Close()
	var out []OnboardingParty
	for rows.Next() {
		var item OnboardingParty
		if err := rows.Scan(
			&item.ID, &item.ApplicationID, &item.MerchantID, &item.PartyType, &item.FullName, &item.Title,
			&item.Email, &item.Phone, &item.OwnershipBPS, &item.VerificationStatus, &item.EvidenceNotes,
			&item.Revision, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Email, err = r.protector.OpenString(item.Email)
		if err != nil {
			return nil, fmt.Errorf("decrypt onboarding party email: %w", err)
		}
		item.Phone, err = r.protector.OpenString(item.Phone)
		if err != nil {
			return nil, fmt.Errorf("decrypt onboarding party phone: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ReplaceOnboardingParties(ctx context.Context, merchantID string, parties []OnboardingParty, actor, actorScope string) ([]OnboardingParty, error) {
	app, err := r.GetOnboardingApplicationByMerchant(ctx, merchantID)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var nextRevision int
	if err := tx.QueryRow(ctx, `
SELECT COALESCE(MAX(revision), 0) + 1
FROM paygate_merchants.merchant_onboarding_parties
WHERE merchant_id = $1
`, merchantID).Scan(&nextRevision); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE paygate_merchants.merchant_onboarding_parties
SET superseded_at = NOW(), updated_at = NOW()
WHERE merchant_id = $1 AND superseded_at IS NULL
`, merchantID); err != nil {
		return nil, fmt.Errorf("supersede onboarding parties: %w", err)
	}
	for _, party := range parties {
		email, err := r.protector.SealString(party.Email)
		if err != nil {
			return nil, fmt.Errorf("encrypt onboarding party email: %w", err)
		}
		phone, err := r.protector.SealString(party.Phone)
		if err != nil {
			return nil, fmt.Errorf("encrypt onboarding party phone: %w", err)
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO paygate_merchants.merchant_onboarding_parties
    (id, application_id, merchant_id, party_type, full_name, title, email, phone, ownership_bps,
     verification_status, evidence_notes, revision)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
`, idgen.New("kybp"), app.ID, merchantID, party.PartyType, party.FullName, party.Title, email, phone, party.OwnershipBPS, party.VerificationStatus, party.EvidenceNotes, nextRevision); err != nil {
			return nil, fmt.Errorf("insert onboarding party: %w", err)
		}
	}
	if err := r.insertOnboardingEventTx(ctx, tx, app.ID, merchantID, "parties.replaced", actor, actorScope, string(app.State), string(app.State), map[string]any{
		"revision": nextRevision,
		"count":    len(parties),
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.ListOnboardingParties(ctx, merchantID)
}

func (r *PostgresRepository) ListOnboardingDocuments(ctx context.Context, merchantID string) ([]OnboardingDocument, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, application_id, merchant_id, document_type, file_name, content_type, storage_key, request_reason, review_notes,
       status, requested_at, uploaded_at, reviewed_at, expires_at, created_at, updated_at
FROM paygate_merchants.merchant_onboarding_documents
WHERE merchant_id = $1
ORDER BY created_at, id
`, merchantID)
	if err != nil {
		return nil, fmt.Errorf("list onboarding documents: %w", err)
	}
	defer rows.Close()
	var out []OnboardingDocument
	for rows.Next() {
		var item OnboardingDocument
		if err := rows.Scan(
			&item.ID, &item.ApplicationID, &item.MerchantID, &item.DocumentType, &item.FileName, &item.ContentType, &item.StorageKey,
			&item.RequestReason, &item.ReviewNotes, &item.Status, &item.RequestedAt, &item.UploadedAt, &item.ReviewedAt,
			&item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.StorageKey, err = r.protector.OpenString(item.StorageKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt onboarding storage key: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) RequestOnboardingDocument(ctx context.Context, merchantID string, doc OnboardingDocument, actor, actorScope string) (OnboardingDocument, error) {
	app, err := r.GetOnboardingApplicationByMerchant(ctx, merchantID)
	if err != nil {
		return OnboardingDocument{}, err
	}
	id := idgen.New("kybd")
	if err := r.db.QueryRow(ctx, `
INSERT INTO paygate_merchants.merchant_onboarding_documents
    (id, application_id, merchant_id, document_type, request_reason, status)
VALUES ($1, $2, $3, $4, $5, 'requested')
RETURNING id, application_id, merchant_id, document_type, file_name, content_type, storage_key, request_reason, review_notes,
          status, requested_at, uploaded_at, reviewed_at, expires_at, created_at, updated_at
`, id, app.ID, merchantID, doc.DocumentType, doc.RequestReason).Scan(
		&doc.ID, &doc.ApplicationID, &doc.MerchantID, &doc.DocumentType, &doc.FileName, &doc.ContentType, &doc.StorageKey, &doc.RequestReason,
		&doc.ReviewNotes, &doc.Status, &doc.RequestedAt, &doc.UploadedAt, &doc.ReviewedAt, &doc.ExpiresAt, &doc.CreatedAt, &doc.UpdatedAt,
	); err != nil {
		return OnboardingDocument{}, fmt.Errorf("request onboarding document: %w", err)
	}
	if err := r.insertOnboardingEvent(ctx, app.ID, merchantID, "document.requested", actor, actorScope, string(app.State), string(app.State), map[string]any{
		"document_id":   doc.ID,
		"document_type": doc.DocumentType,
		"reason":        doc.RequestReason,
	}); err != nil {
		return OnboardingDocument{}, err
	}
	return doc, nil
}

func (r *PostgresRepository) UploadOnboardingDocument(ctx context.Context, merchantID string, doc OnboardingDocument, actor, actorScope string) (OnboardingDocument, error) {
	app, err := r.GetOnboardingApplicationByMerchant(ctx, merchantID)
	if err != nil {
		return OnboardingDocument{}, err
	}
	now := time.Now().UTC()
	encryptedStorageKey, err := r.protector.SealString(doc.StorageKey)
	if err != nil {
		return OnboardingDocument{}, fmt.Errorf("encrypt storage key: %w", err)
	}
	if doc.ID != "" {
		if err := r.db.QueryRow(ctx, `
UPDATE paygate_merchants.merchant_onboarding_documents
SET file_name = $3,
    content_type = $4,
    storage_key = $5,
    status = 'uploaded',
    uploaded_at = $6,
    updated_at = $6
WHERE id = $1 AND merchant_id = $2
RETURNING id, application_id, merchant_id, document_type, file_name, content_type, storage_key, request_reason, review_notes,
          status, requested_at, uploaded_at, reviewed_at, expires_at, created_at, updated_at
`, doc.ID, merchantID, doc.FileName, doc.ContentType, encryptedStorageKey, now).Scan(
			&doc.ID, &doc.ApplicationID, &doc.MerchantID, &doc.DocumentType, &doc.FileName, &doc.ContentType, &doc.StorageKey, &doc.RequestReason,
			&doc.ReviewNotes, &doc.Status, &doc.RequestedAt, &doc.UploadedAt, &doc.ReviewedAt, &doc.ExpiresAt, &doc.CreatedAt, &doc.UpdatedAt,
		); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return OnboardingDocument{}, ErrOnboardingApplicationNotFound
			}
			return OnboardingDocument{}, fmt.Errorf("upload onboarding document: %w", err)
		}
	} else {
		doc.ID = idgen.New("kybd")
		if err := r.db.QueryRow(ctx, `
INSERT INTO paygate_merchants.merchant_onboarding_documents
    (id, application_id, merchant_id, document_type, file_name, content_type, storage_key, status, requested_at, uploaded_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'uploaded', $8, $8)
RETURNING id, application_id, merchant_id, document_type, file_name, content_type, storage_key, request_reason, review_notes,
          status, requested_at, uploaded_at, reviewed_at, expires_at, created_at, updated_at
`, doc.ID, app.ID, merchantID, doc.DocumentType, doc.FileName, doc.ContentType, encryptedStorageKey, now).Scan(
			&doc.ID, &doc.ApplicationID, &doc.MerchantID, &doc.DocumentType, &doc.FileName, &doc.ContentType, &doc.StorageKey, &doc.RequestReason,
			&doc.ReviewNotes, &doc.Status, &doc.RequestedAt, &doc.UploadedAt, &doc.ReviewedAt, &doc.ExpiresAt, &doc.CreatedAt, &doc.UpdatedAt,
		); err != nil {
			return OnboardingDocument{}, fmt.Errorf("insert onboarding document upload: %w", err)
		}
	}
	doc.StorageKey, err = r.protector.OpenString(doc.StorageKey)
	if err != nil {
		return OnboardingDocument{}, fmt.Errorf("decrypt storage key: %w", err)
	}
	if err := r.insertOnboardingEvent(ctx, app.ID, merchantID, "document.uploaded", actor, actorScope, string(app.State), string(app.State), map[string]any{
		"document_id":   doc.ID,
		"document_type": doc.DocumentType,
		"storage_key":   "[protected]",
	}); err != nil {
		return OnboardingDocument{}, err
	}
	return doc, nil
}

func (r *PostgresRepository) ReviewOnboardingDocument(ctx context.Context, merchantID, documentID string, status DocumentStatus, reviewNotes string, expiresAt *time.Time, actor, actorScope string) (OnboardingDocument, error) {
	app, err := r.GetOnboardingApplicationByMerchant(ctx, merchantID)
	if err != nil {
		return OnboardingDocument{}, err
	}
	var doc OnboardingDocument
	if err := r.db.QueryRow(ctx, `
UPDATE paygate_merchants.merchant_onboarding_documents
SET status = $3,
    review_notes = $4,
    reviewed_at = NOW(),
    expires_at = $5,
    updated_at = NOW()
WHERE id = $1 AND merchant_id = $2
RETURNING id, application_id, merchant_id, document_type, file_name, content_type, storage_key, request_reason, review_notes,
          status, requested_at, uploaded_at, reviewed_at, expires_at, created_at, updated_at
`, documentID, merchantID, status, reviewNotes, expiresAt).Scan(
		&doc.ID, &doc.ApplicationID, &doc.MerchantID, &doc.DocumentType, &doc.FileName, &doc.ContentType, &doc.StorageKey, &doc.RequestReason,
		&doc.ReviewNotes, &doc.Status, &doc.RequestedAt, &doc.UploadedAt, &doc.ReviewedAt, &doc.ExpiresAt, &doc.CreatedAt, &doc.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OnboardingDocument{}, ErrOnboardingApplicationNotFound
		}
		return OnboardingDocument{}, fmt.Errorf("review onboarding document: %w", err)
	}
	doc.StorageKey, err = r.protector.OpenString(doc.StorageKey)
	if err != nil {
		return OnboardingDocument{}, fmt.Errorf("decrypt storage key: %w", err)
	}
	if status == DocumentStatusExpired {
		if _, err := r.db.Exec(ctx, `
UPDATE paygate_merchants.merchant_onboarding_applications
SET state = CASE WHEN state = 'approved' THEN 'needs_information' ELSE state END,
    updated_at = NOW()
WHERE merchant_id = $1
`, merchantID); err != nil {
			return OnboardingDocument{}, err
		}
	}
	if err := r.insertOnboardingEvent(ctx, app.ID, merchantID, "document.reviewed", actor, actorScope, string(app.State), string(app.State), map[string]any{
		"document_id":   doc.ID,
		"document_type": doc.DocumentType,
		"status":        doc.Status,
	}); err != nil {
		return OnboardingDocument{}, err
	}
	return doc, nil
}

func (r *PostgresRepository) ListScreeningCases(ctx context.Context, merchantID string) ([]ScreeningCase, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, application_id, merchant_id, screening_type, provider, provider_reference, subject_name, status, result_payload,
       reviewed_by, screened_at, reviewed_at, created_at
FROM paygate_merchants.merchant_screening_cases
WHERE merchant_id = $1
ORDER BY created_at DESC, id DESC
`, merchantID)
	if err != nil {
		return nil, fmt.Errorf("list screening cases: %w", err)
	}
	defer rows.Close()
	var out []ScreeningCase
	for rows.Next() {
		var item ScreeningCase
		var raw []byte
		if err := rows.Scan(&item.ID, &item.ApplicationID, &item.MerchantID, &item.ScreeningType, &item.Provider, &item.ProviderReference,
			&item.SubjectName, &item.Status, &raw, &item.ReviewedBy, &item.ScreenedAt, &item.ReviewedAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &item.ResultPayload); err != nil {
				return nil, err
			}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CreateScreeningCase(ctx context.Context, merchantID string, screening ScreeningCase, actor, actorScope string) (ScreeningCase, error) {
	app, err := r.GetOnboardingApplicationByMerchant(ctx, merchantID)
	if err != nil {
		return ScreeningCase{}, err
	}
	raw, err := json.Marshal(screening.ResultPayload)
	if err != nil {
		return ScreeningCase{}, err
	}
	screening.ID = idgen.New("scr")
	screening.ApplicationID = app.ID
	screening.MerchantID = merchantID
	if err := r.db.QueryRow(ctx, `
INSERT INTO paygate_merchants.merchant_screening_cases
    (id, application_id, merchant_id, screening_type, provider, provider_reference, subject_name, status, result_payload, reviewed_by, screened_at, reviewed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING created_at
`, screening.ID, screening.ApplicationID, merchantID, screening.ScreeningType, screening.Provider, screening.ProviderReference,
		screening.SubjectName, screening.Status, raw, screening.ReviewedBy, screening.ScreenedAt, screening.ReviewedAt).Scan(&screening.CreatedAt); err != nil {
		return ScreeningCase{}, fmt.Errorf("create screening case: %w", err)
	}
	if screening.Status == ScreeningStatusReview || screening.Status == ScreeningStatusFailed {
		if _, err := r.UpsertCapabilities(ctx, merchantID, []MerchantCapability{
			{CapabilityCode: CapabilityPayments, Status: CapabilityStatusRestricted, Reason: "screening review required"},
			{CapabilityCode: CapabilityPayouts, Status: CapabilityStatusRestricted, Reason: "screening review required"},
		}, actor); err != nil {
			return ScreeningCase{}, err
		}
	}
	if err := r.insertOnboardingEvent(ctx, app.ID, merchantID, "screening.created", actor, actorScope, string(app.State), string(app.State), map[string]any{
		"screening_id": screening.ID,
		"status":       screening.Status,
		"type":         screening.ScreeningType,
	}); err != nil {
		return ScreeningCase{}, err
	}
	return screening, nil
}

func (r *PostgresRepository) ListCapabilities(ctx context.Context, merchantID string) ([]MerchantCapability, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, capability_code, status, reason, updated_by, created_at, updated_at
FROM paygate_merchants.merchant_capabilities
WHERE merchant_id = $1
ORDER BY capability_code
`, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MerchantCapability
	for rows.Next() {
		var item MerchantCapability
		if err := rows.Scan(&item.ID, &item.MerchantID, &item.CapabilityCode, &item.Status, &item.Reason, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return DefaultCapabilities(merchantID), nil
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpsertCapabilities(ctx context.Context, merchantID string, capabilities []MerchantCapability, actor string) ([]MerchantCapability, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, capability := range capabilities {
		if _, err := tx.Exec(ctx, `
INSERT INTO paygate_merchants.merchant_capabilities
    (id, merchant_id, capability_code, status, reason, updated_by)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (merchant_id, capability_code)
DO UPDATE SET status = EXCLUDED.status, reason = EXCLUDED.reason, updated_by = EXCLUDED.updated_by, updated_at = NOW()
`, idgen.New("mcap"), merchantID, capability.CapabilityCode, capability.Status, capability.Reason, actor); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.ListCapabilities(ctx, merchantID)
}

func (r *PostgresRepository) GetReservePolicy(ctx context.Context, merchantID string) (ReservePolicy, error) {
	var policy ReservePolicy
	err := r.db.QueryRow(ctx, `
SELECT id, merchant_id, policy_type, percentage_bps, hold_days, threshold_amount, notes, created_at, updated_at
FROM paygate_merchants.merchant_reserve_policies
WHERE merchant_id = $1
`, merchantID).Scan(&policy.ID, &policy.MerchantID, &policy.PolicyType, &policy.PercentageBPS, &policy.HoldDays, &policy.ThresholdAmount, &policy.Notes, &policy.CreatedAt, &policy.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DefaultReservePolicy(merchantID), nil
		}
		return ReservePolicy{}, err
	}
	return policy, nil
}

func (r *PostgresRepository) UpsertReservePolicy(ctx context.Context, policy ReservePolicy, actor string) (ReservePolicy, error) {
	if err := r.db.QueryRow(ctx, `
INSERT INTO paygate_merchants.merchant_reserve_policies
    (id, merchant_id, policy_type, percentage_bps, hold_days, threshold_amount, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (merchant_id)
DO UPDATE SET policy_type = EXCLUDED.policy_type,
              percentage_bps = EXCLUDED.percentage_bps,
              hold_days = EXCLUDED.hold_days,
              threshold_amount = EXCLUDED.threshold_amount,
              notes = EXCLUDED.notes,
              updated_at = NOW()
RETURNING id, merchant_id, policy_type, percentage_bps, hold_days, threshold_amount, notes, created_at, updated_at
`, idgen.New("mrsv"), policy.MerchantID, policy.PolicyType, policy.PercentageBPS, policy.HoldDays, policy.ThresholdAmount, policy.Notes).Scan(
		&policy.ID, &policy.MerchantID, &policy.PolicyType, &policy.PercentageBPS, &policy.HoldDays, &policy.ThresholdAmount, &policy.Notes, &policy.CreatedAt, &policy.UpdatedAt,
	); err != nil {
		return ReservePolicy{}, err
	}
	app, err := r.GetOnboardingApplicationByMerchant(ctx, policy.MerchantID)
	if err == nil {
		_ = r.insertOnboardingEvent(ctx, app.ID, policy.MerchantID, "reserve_policy.updated", actor, "admin", string(app.State), string(app.State), map[string]any{
			"policy_type":      policy.PolicyType,
			"percentage_bps":   policy.PercentageBPS,
			"hold_days":        policy.HoldDays,
			"threshold_amount": policy.ThresholdAmount,
		})
	}
	return policy, nil
}

func scanReserveEscalation(row interface{ Scan(dest ...any) error }) (ReserveEscalation, error) {
	var item ReserveEscalation
	var triggeredRules []byte
	if err := row.Scan(&item.ID, &item.MerchantID, &item.RiskEventID, &item.TriggerScore, &triggeredRules, &item.Status,
		&item.SuggestedPolicyType, &item.SuggestedPercentageBPS, &item.SuggestedHoldDays, &item.SuggestedThresholdAmount,
		&item.Rationale, &item.ReviewNotes, &item.ReviewedBy, &item.ReviewedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return ReserveEscalation{}, err
	}
	_ = json.Unmarshal(triggeredRules, &item.TriggeredRules)
	return item, nil
}

func (r *PostgresRepository) CreateReserveEscalation(ctx context.Context, escalation ReserveEscalation) (ReserveEscalation, error) {
	escalation.ID = idgen.New("resc")
	triggeredRules, _ := json.Marshal(escalation.TriggeredRules)
	err := r.db.QueryRow(ctx, `
INSERT INTO paygate_merchants.reserve_escalations
    (id, merchant_id, risk_event_id, trigger_score, triggered_rules, status, suggested_policy_type, suggested_percentage_bps, suggested_hold_days, suggested_threshold_amount, rationale)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING id, merchant_id, risk_event_id, trigger_score, triggered_rules, status, suggested_policy_type, suggested_percentage_bps, suggested_hold_days, suggested_threshold_amount, rationale, review_notes, reviewed_by, reviewed_at, created_at, updated_at
`, escalation.ID, escalation.MerchantID, escalation.RiskEventID, escalation.TriggerScore, triggeredRules, escalation.Status, escalation.SuggestedPolicyType,
		escalation.SuggestedPercentageBPS, escalation.SuggestedHoldDays, escalation.SuggestedThresholdAmount, escalation.Rationale).Scan(
		&escalation.ID, &escalation.MerchantID, &escalation.RiskEventID, &escalation.TriggerScore, &triggeredRules, &escalation.Status,
		&escalation.SuggestedPolicyType, &escalation.SuggestedPercentageBPS, &escalation.SuggestedHoldDays, &escalation.SuggestedThresholdAmount,
		&escalation.Rationale, &escalation.ReviewNotes, &escalation.ReviewedBy, &escalation.ReviewedAt, &escalation.CreatedAt, &escalation.UpdatedAt)
	if err != nil {
		return ReserveEscalation{}, err
	}
	_ = json.Unmarshal(triggeredRules, &escalation.TriggeredRules)
	return escalation, nil
}

func (r *PostgresRepository) ListReserveEscalations(ctx context.Context, merchantID string, status ReserveEscalationStatus) ([]ReserveEscalation, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, risk_event_id, trigger_score, triggered_rules, status, suggested_policy_type, suggested_percentage_bps, suggested_hold_days, suggested_threshold_amount, rationale, review_notes, reviewed_by, reviewed_at, created_at, updated_at
FROM paygate_merchants.reserve_escalations
WHERE merchant_id = $1 AND ($2 = '' OR status = $2)
ORDER BY created_at DESC
`, merchantID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReserveEscalation
	for rows.Next() {
		item, err := scanReserveEscalation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ReviewReserveEscalation(ctx context.Context, merchantID, escalationID string, decision ReserveEscalationStatus, notes, actor string, policyOverride *ReservePolicy) (ReserveEscalation, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return ReserveEscalation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var escalation ReserveEscalation
	escalation, err = scanReserveEscalation(tx.QueryRow(ctx, `
SELECT id, merchant_id, risk_event_id, trigger_score, triggered_rules, status, suggested_policy_type, suggested_percentage_bps, suggested_hold_days, suggested_threshold_amount, rationale, review_notes, reviewed_by, reviewed_at, created_at, updated_at
FROM paygate_merchants.reserve_escalations
WHERE merchant_id = $1 AND id = $2
FOR UPDATE
`, merchantID, escalationID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ReserveEscalation{}, ErrReserveEscalationNotFound
		}
		return ReserveEscalation{}, err
	}
	if decision == ReserveEscalationApproved {
		policy := ReservePolicy{
			MerchantID:      merchantID,
			PolicyType:      escalation.SuggestedPolicyType,
			PercentageBPS:   escalation.SuggestedPercentageBPS,
			HoldDays:        escalation.SuggestedHoldDays,
			ThresholdAmount: escalation.SuggestedThresholdAmount,
			Notes:           "reserve escalation approved",
		}
		if policyOverride != nil {
			policy = *policyOverride
		}
		if err := tx.QueryRow(ctx, `
INSERT INTO paygate_merchants.merchant_reserve_policies
    (id, merchant_id, policy_type, percentage_bps, hold_days, threshold_amount, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (merchant_id)
DO UPDATE SET policy_type = EXCLUDED.policy_type, percentage_bps = EXCLUDED.percentage_bps, hold_days = EXCLUDED.hold_days, threshold_amount = EXCLUDED.threshold_amount, notes = EXCLUDED.notes, updated_at = NOW()
RETURNING id, merchant_id, policy_type, percentage_bps, hold_days, threshold_amount, notes, created_at, updated_at
`, idgen.New("mrsv"), policy.MerchantID, policy.PolicyType, policy.PercentageBPS, policy.HoldDays, policy.ThresholdAmount, policy.Notes).Scan(
			&policy.ID, &policy.MerchantID, &policy.PolicyType, &policy.PercentageBPS, &policy.HoldDays, &policy.ThresholdAmount, &policy.Notes, &policy.CreatedAt, &policy.UpdatedAt,
		); err != nil {
			return ReserveEscalation{}, err
		}
	}
	escalation, err = scanReserveEscalation(tx.QueryRow(ctx, `
UPDATE paygate_merchants.reserve_escalations
SET status = $3, review_notes = $4, reviewed_by = $5, reviewed_at = NOW(), updated_at = NOW()
WHERE merchant_id = $1 AND id = $2
RETURNING id, merchant_id, risk_event_id, trigger_score, triggered_rules, status, suggested_policy_type, suggested_percentage_bps, suggested_hold_days, suggested_threshold_amount, rationale, review_notes, reviewed_by, reviewed_at, created_at, updated_at
`, merchantID, escalationID, decision, notes, actor))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ReserveEscalation{}, ErrReserveEscalationNotFound
		}
		return ReserveEscalation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReserveEscalation{}, err
	}
	return escalation, nil
}
