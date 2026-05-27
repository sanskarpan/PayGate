package payout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sanskarpan/PayGate/internal/common/idgen"
	"github.com/sanskarpan/PayGate/internal/common/protect"
)

func (r *PostgresRepository) ListBeneficiaries(ctx context.Context, merchantID string) ([]Beneficiary, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, destination_type, account_holder_name, bank_account_last4, bank_ifsc, vpa, fingerprint,
       status, verification_fresh_until, approved_at, approval_notes, created_at, updated_at
FROM paygate_payouts.beneficiaries
WHERE merchant_id = $1
ORDER BY created_at DESC
`, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Beneficiary
	for rows.Next() {
		item, err := scanBeneficiary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetBeneficiary(ctx context.Context, merchantID, beneficiaryID string) (Beneficiary, error) {
	row := r.db.QueryRow(ctx, `
SELECT id, merchant_id, destination_type, account_holder_name, bank_account_last4, bank_ifsc, vpa, fingerprint,
       status, verification_fresh_until, approved_at, approval_notes, created_at, updated_at
FROM paygate_payouts.beneficiaries
WHERE merchant_id = $1 AND id = $2
`, merchantID, beneficiaryID)
	item, err := scanBeneficiary(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Beneficiary{}, ErrBeneficiaryNotFound
	}
	return item, err
}

func (r *PostgresRepository) CreateBeneficiary(ctx context.Context, beneficiary Beneficiary, actor, actorScope string) (Beneficiary, error) {
	beneficiary.ID = idgen.New("bene")
	beneficiary.Fingerprint = fingerprintBeneficiary(beneficiary)
	accountHolderName, err := protect.Default().SealStringForDomain(protect.DomainPayoutBeneficiaryIdentity, beneficiary.AccountHolderName)
	if err != nil {
		return Beneficiary{}, err
	}
	bankIFSC, err := protect.Default().SealStringForDomain(protect.DomainPayoutBeneficiaryIdentity, beneficiary.BankIFSC)
	if err != nil {
		return Beneficiary{}, err
	}
	vpa, err := protect.Default().SealStringForDomain(protect.DomainPayoutBeneficiaryIdentity, beneficiary.VPA)
	if err != nil {
		return Beneficiary{}, err
	}
	if err := r.db.QueryRow(ctx, `
INSERT INTO paygate_payouts.beneficiaries
    (id, merchant_id, destination_type, account_holder_name, bank_account_last4, bank_ifsc, vpa, fingerprint, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending_verification')
RETURNING id, merchant_id, destination_type, account_holder_name, bank_account_last4, bank_ifsc, vpa, fingerprint,
          status, verification_fresh_until, approved_at, approval_notes, created_at, updated_at
`, beneficiary.ID, beneficiary.MerchantID, beneficiary.DestinationType, accountHolderName, beneficiary.BankAccountLast4, bankIFSC, vpa, beneficiary.Fingerprint).Scan(
		&beneficiary.ID, &beneficiary.MerchantID, &beneficiary.DestinationType, &beneficiary.AccountHolderName, &beneficiary.BankAccountLast4,
		&beneficiary.BankIFSC, &beneficiary.VPA, &beneficiary.Fingerprint, &beneficiary.Status, &beneficiary.VerificationFreshUntil,
		&beneficiary.ApprovedAt, &beneficiary.ApprovalNotes, &beneficiary.CreatedAt, &beneficiary.UpdatedAt,
	); err != nil {
		return Beneficiary{}, err
	}
	return beneficiary, r.recordBeneficiaryEvent(ctx, beneficiary.ID, beneficiary.MerchantID, "beneficiary.created", actor, actorScope, map[string]any{
		"destination_type": beneficiary.DestinationType,
	})
}

func (r *PostgresRepository) VerifyBeneficiary(ctx context.Context, merchantID, beneficiaryID string, evidence map[string]any) (Beneficiary, BeneficiaryVerification, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Beneficiary{}, BeneficiaryVerification{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := r.getBeneficiaryForUpdate(ctx, tx, merchantID, beneficiaryID)
	if err != nil {
		return Beneficiary{}, BeneficiaryVerification{}, err
	}
	verifiedAt := time.Now().UTC()
	freshUntil := verifiedAt.Add(30 * 24 * time.Hour)
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payouts.beneficiaries
SET status = 'verified', verification_fresh_until = $3, updated_at = $2
WHERE id = $1
`, beneficiaryID, verifiedAt, freshUntil); err != nil {
		return Beneficiary{}, BeneficiaryVerification{}, err
	}
	raw, _ := json.Marshal(evidence)
	provider := "simulated"
	providerReference := "verify_" + beneficiaryID
	if method, _ := evidence["method"].(string); method != "" {
		switch method {
		case "penny_drop":
			provider = "penny_drop"
			providerReference = "pdrop_" + beneficiaryID
		case "upi_payee_verification":
			provider = "upi_payee_verification"
			providerReference = "upivpa_" + beneficiaryID
		}
	}
	verification := BeneficiaryVerification{
		ID:                idgen.New("bver"),
		BeneficiaryID:     beneficiaryID,
		MerchantID:        merchantID,
		Provider:          provider,
		ProviderReference: providerReference,
		Status:            "passed",
		Evidence:          evidence,
		VerifiedAt:        &verifiedAt,
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_payouts.beneficiary_verifications
    (id, beneficiary_id, merchant_id, provider, provider_reference, status, evidence_json, verified_at)
VALUES ($1, $2, $3, $4, $5, 'passed', $6, $7)
`, verification.ID, beneficiaryID, merchantID, verification.Provider, verification.ProviderReference, raw, verifiedAt); err != nil {
		return Beneficiary{}, BeneficiaryVerification{}, err
	}
	if err := r.recordBeneficiaryEventTx(ctx, tx, beneficiaryID, merchantID, "beneficiary.verified", "system", "verification", map[string]any{
		"verification_id": verification.ID,
	}); err != nil {
		return Beneficiary{}, BeneficiaryVerification{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Beneficiary{}, BeneficiaryVerification{}, err
	}
	current.Status = BeneficiaryStatusVerified
	current.VerificationFreshUntil = &freshUntil
	return current, verification, nil
}

func (r *PostgresRepository) ApproveBeneficiary(ctx context.Context, merchantID, beneficiaryID, notes, actor, actorScope string) (Beneficiary, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Beneficiary{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := r.getBeneficiaryForUpdate(ctx, tx, merchantID, beneficiaryID)
	if err != nil {
		return Beneficiary{}, err
	}
	if current.Status != BeneficiaryStatusVerified && current.Status != BeneficiaryStatusApproved {
		return Beneficiary{}, ErrBeneficiaryNotApproved
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payouts.beneficiaries
SET status = 'approved', approval_notes = $3, approved_at = $4, updated_at = $4
WHERE id = $1 AND merchant_id = $2
`, beneficiaryID, merchantID, notes, now); err != nil {
		return Beneficiary{}, err
	}
	if err := r.recordBeneficiaryEventTx(ctx, tx, beneficiaryID, merchantID, "beneficiary.approved", actor, actorScope, map[string]any{
		"notes": notes,
	}); err != nil {
		return Beneficiary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Beneficiary{}, err
	}
	return r.GetBeneficiary(ctx, merchantID, beneficiaryID)
}

func (r *PostgresRepository) RecordApproval(ctx context.Context, merchantID, payoutID, actor, actorScope, decision, notes string) (Payout, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Payout{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := r.getPayoutForUpdate(ctx, tx, merchantID, payoutID)
	if err != nil {
		return Payout{}, err
	}
	if current.ApprovalStatus == ApprovalStatusNotRequired {
		return current, nil
	}
	nextStatus := ApprovalStatusApproved
	if decision == "rejected" {
		nextStatus = ApprovalStatusRejected
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
UPDATE paygate_payouts.payouts
SET approval_status = $3, approval_notes = $4, approved_by = $5, approved_at = $6, updated_at = $6
WHERE id = $1 AND merchant_id = $2
`, payoutID, merchantID, nextStatus, notes, actor, now); err != nil {
		return Payout{}, err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_payouts.payout_approvals
    (id, payout_id, merchant_id, actor, actor_scope, decision, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`, idgen.New("papr"), payoutID, merchantID, actor, actorScope, decision, notes); err != nil {
		return Payout{}, err
	}
	if decision == "rejected" && current.Status == StatePending {
		if _, err := tx.Exec(ctx, `
UPDATE paygate_payouts.payouts
SET status = 'cancelled', cancel_reason = $3, cancelled_at = $4, updated_at = $4
WHERE id = $1 AND merchant_id = $2
`, payoutID, merchantID, "approval rejected", now); err != nil {
			return Payout{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Payout{}, err
	}
	return r.GetByID(ctx, merchantID, payoutID)
}

func (r *PostgresRepository) ListApprovals(ctx context.Context, merchantID, payoutID string) ([]ApprovalRecord, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, payout_id, merchant_id, actor, actor_scope, decision, notes, created_at
FROM paygate_payouts.payout_approvals
WHERE merchant_id = $1 AND payout_id = $2
ORDER BY created_at
`, merchantID, payoutID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ApprovalRecord
	for rows.Next() {
		var item ApprovalRecord
		if err := rows.Scan(&item.ID, &item.PayoutID, &item.MerchantID, &item.Actor, &item.ActorScope, &item.Decision, &item.Notes, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CreateBatch(ctx context.Context, batch Batch, items []BatchItem) (Batch, []BatchItem, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Batch{}, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	batch.ID = idgen.New("pob")
	if _, err := tx.Exec(ctx, `
INSERT INTO paygate_payouts.payout_batches
    (id, merchant_id, dry_run, status, idempotency_key, summary_json)
VALUES ($1, $2, $3, $4, $5, '{}'::jsonb)
`, batch.ID, batch.MerchantID, batch.DryRun, batch.Status, batch.IdempotencyKey); err != nil {
		return Batch{}, nil, err
	}
	for i := range items {
		items[i].ID = idgen.New("pobi")
		items[i].BatchID = batch.ID
		if items[i].Status == "" {
			items[i].Status = "preview"
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO paygate_payouts.payout_batch_items
    (id, batch_id, merchant_id, settlement_id, beneficiary_id, payout_id, status, error_text)
VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8)
`, items[i].ID, batch.ID, items[i].MerchantID, items[i].SettlementID, items[i].BeneficiaryID, items[i].PayoutID, items[i].Status, items[i].ErrorText); err != nil {
			return Batch{}, nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Batch{}, nil, err
	}
	batch, err = r.FinalizeBatch(ctx, batch.ID, batch.Status, map[string]any{"item_count": len(items)})
	if err != nil {
		return Batch{}, nil, err
	}
	return batch, items, nil
}

func (r *PostgresRepository) ListBatches(ctx context.Context, merchantID string, limit int) ([]Batch, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, merchant_id, dry_run, status, idempotency_key, summary_json, created_at, updated_at
FROM paygate_payouts.payout_batches
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2
`, merchantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Batch
	for rows.Next() {
		var item Batch
		var raw []byte
		if err := rows.Scan(&item.ID, &item.MerchantID, &item.DryRun, &item.Status, &item.IdempotencyKey, &raw, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &item.Summary)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetBatch(ctx context.Context, merchantID, batchID string) (Batch, error) {
	var item Batch
	var raw []byte
	if err := r.db.QueryRow(ctx, `
SELECT id, merchant_id, dry_run, status, idempotency_key, summary_json, created_at, updated_at
FROM paygate_payouts.payout_batches
WHERE merchant_id = $1 AND id = $2
`, merchantID, batchID).Scan(&item.ID, &item.MerchantID, &item.DryRun, &item.Status, &item.IdempotencyKey, &raw, &item.CreatedAt, &item.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Batch{}, ErrPayoutBatchNotFound
		}
		return Batch{}, err
	}
	_ = json.Unmarshal(raw, &item.Summary)
	return item, nil
}

func (r *PostgresRepository) ListBatchItems(ctx context.Context, merchantID, batchID string) ([]BatchItem, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, batch_id, merchant_id, settlement_id, beneficiary_id, COALESCE(payout_id, ''), status, error_text, created_at
FROM paygate_payouts.payout_batch_items
WHERE merchant_id = $1 AND batch_id = $2
ORDER BY created_at, id
`, merchantID, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BatchItem
	for rows.Next() {
		var item BatchItem
		if err := rows.Scan(&item.ID, &item.BatchID, &item.MerchantID, &item.SettlementID, &item.BeneficiaryID, &item.PayoutID, &item.Status, &item.ErrorText, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpdateBatchItem(ctx context.Context, batchItemID, payoutID, status, errorText string) error {
	_, err := r.db.Exec(ctx, `
UPDATE paygate_payouts.payout_batch_items
SET payout_id = NULLIF($2, ''),
    status = $3,
    error_text = $4
WHERE id = $1
`, batchItemID, payoutID, status, errorText)
	return err
}

func (r *PostgresRepository) FinalizeBatch(ctx context.Context, batchID, status string, summary map[string]any) (Batch, error) {
	raw, err := json.Marshal(summary)
	if err != nil {
		return Batch{}, err
	}
	var batch Batch
	if err := r.db.QueryRow(ctx, `
UPDATE paygate_payouts.payout_batches
SET status = $2, summary_json = $3, updated_at = NOW()
WHERE id = $1
RETURNING id, merchant_id, dry_run, status, idempotency_key, summary_json, created_at, updated_at
`, batchID, status, raw).Scan(&batch.ID, &batch.MerchantID, &batch.DryRun, &batch.Status, &batch.IdempotencyKey, &raw, &batch.CreatedAt, &batch.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Batch{}, ErrPayoutBatchNotFound
		}
		return Batch{}, err
	}
	_ = json.Unmarshal(raw, &batch.Summary)
	return batch, nil
}

func scanBeneficiary(row scannable) (Beneficiary, error) {
	var item Beneficiary
	err := row.Scan(&item.ID, &item.MerchantID, &item.DestinationType, &item.AccountHolderName, &item.BankAccountLast4, &item.BankIFSC, &item.VPA, &item.Fingerprint, &item.Status, &item.VerificationFreshUntil, &item.ApprovedAt, &item.ApprovalNotes, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	item.AccountHolderName, err = protect.Default().OpenStringForDomain(protect.DomainPayoutBeneficiaryIdentity, item.AccountHolderName)
	if err != nil {
		return Beneficiary{}, err
	}
	item.BankIFSC, err = protect.Default().OpenStringForDomain(protect.DomainPayoutBeneficiaryIdentity, item.BankIFSC)
	if err != nil {
		return Beneficiary{}, err
	}
	item.VPA, err = protect.Default().OpenStringForDomain(protect.DomainPayoutBeneficiaryIdentity, item.VPA)
	if err != nil {
		return Beneficiary{}, err
	}
	return item, nil
}

func (r *PostgresRepository) getBeneficiaryForUpdate(ctx context.Context, tx pgx.Tx, merchantID, beneficiaryID string) (Beneficiary, error) {
	row := tx.QueryRow(ctx, `
SELECT id, merchant_id, destination_type, account_holder_name, bank_account_last4, bank_ifsc, vpa, fingerprint,
       status, verification_fresh_until, approved_at, approval_notes, created_at, updated_at
FROM paygate_payouts.beneficiaries
WHERE merchant_id = $1 AND id = $2
FOR UPDATE
`, merchantID, beneficiaryID)
	item, err := scanBeneficiary(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Beneficiary{}, ErrBeneficiaryNotFound
	}
	return item, err
}

func (r *PostgresRepository) recordBeneficiaryEvent(ctx context.Context, beneficiaryID, merchantID, eventType, actor, actorScope string, payload map[string]any) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := r.recordBeneficiaryEventTx(ctx, tx, beneficiaryID, merchantID, eventType, actor, actorScope, payload); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) recordBeneficiaryEventTx(ctx context.Context, tx pgx.Tx, beneficiaryID, merchantID, eventType, actor, actorScope string, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO paygate_payouts.beneficiary_events
    (id, beneficiary_id, merchant_id, event_type, actor, actor_scope, payload_json)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`, idgen.New("bevt"), beneficiaryID, merchantID, eventType, actor, actorScope, raw)
	return err
}

func fingerprintBeneficiary(beneficiary Beneficiary) string {
	base := strings.Join([]string{
		string(beneficiary.DestinationType),
		strings.TrimSpace(strings.ToLower(beneficiary.AccountHolderName)),
		strings.TrimSpace(strings.ToUpper(beneficiary.BankIFSC)),
		strings.TrimSpace(strings.ToLower(beneficiary.VPA)),
		strings.TrimSpace(beneficiary.BankAccountLast4),
	}, "|")
	sum := sha256.Sum256([]byte(base))
	return hex.EncodeToString(sum[:])
}
