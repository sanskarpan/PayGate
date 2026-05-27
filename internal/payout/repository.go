package payout

import "context"

// Repository defines storage operations for the payout service.
type Repository interface {
	// CreateForSettlement creates a new payout in pending state for the given settlement.
	CreateForSettlement(ctx context.Context, merchantID, settlementID, beneficiaryID string, amount int64, currency string, approvalStatus ApprovalStatus, batchID string) (Payout, error)

	// GetByID returns a payout by ID scoped to the merchant.
	GetByID(ctx context.Context, merchantID, id string) (Payout, error)

	// GetBySettlementID returns the payout for a settlement scoped to the merchant.
	GetBySettlementID(ctx context.Context, merchantID, settlementID string) (Payout, error)

	// List returns the most recent payouts for a merchant up to limit.
	List(ctx context.Context, merchantID string, limit int) ([]Payout, error)

	// Initiate transitions a payout from pending to processing.
	Initiate(ctx context.Context, merchantID, id string) (Payout, error)

	// Complete transitions a payout from processing to completed and records the bank reference.
	Complete(ctx context.Context, merchantID, id, bankReference string) (Payout, error)

	// Fail transitions a payout from processing to failed and records the failure reason.
	Fail(ctx context.Context, merchantID, id, reason string) (Payout, error)

	// Cancel transitions an in-flight payout to cancelled to prevent further processing.
	Cancel(ctx context.Context, merchantID, id, reason string) (Payout, error)

	// ApplyRailCallback applies a signed rail callback exactly once.
	ApplyRailCallback(ctx context.Context, callback RailCallback, signature string) (Payout, bool, error)

	// ListEvents returns payout timeline events, newest first.
	ListEvents(ctx context.Context, merchantID, payoutID string, limit int) ([]TimelineEvent, error)

	// AttachSaga associates a saga instance with the payout.
	AttachSaga(ctx context.Context, merchantID, id, sagaID string) (Payout, error)

	// UpsertSimulatorScenario configures a payout rail simulation script for a settlement.
	UpsertSimulatorScenario(ctx context.Context, merchantID, settlementID string, scenario SimulatorScenario) (SimulatorScenario, error)

	// GetSimulatorScenario returns the simulator scenario for a settlement.
	GetSimulatorScenario(ctx context.Context, merchantID, settlementID string) (SimulatorScenario, error)

	// GetSimulatorScenarioForPayout returns the simulator scenario tied to the payout settlement and consumes one transient failure if configured.
	GetSimulatorScenarioForPayout(ctx context.Context, merchantID, payoutID string) (SimulatorScenario, bool, error)

	ListBeneficiaries(ctx context.Context, merchantID string) ([]Beneficiary, error)
	GetBeneficiary(ctx context.Context, merchantID, beneficiaryID string) (Beneficiary, error)
	CreateBeneficiary(ctx context.Context, beneficiary Beneficiary, actor, actorScope string) (Beneficiary, error)
	VerifyBeneficiary(ctx context.Context, merchantID, beneficiaryID string, evidence map[string]any) (Beneficiary, BeneficiaryVerification, error)
	ApproveBeneficiary(ctx context.Context, merchantID, beneficiaryID, notes, actor, actorScope string) (Beneficiary, error)
	RecordApproval(ctx context.Context, merchantID, payoutID, actor, actorScope, decision, notes string) (Payout, error)
	ListApprovals(ctx context.Context, merchantID, payoutID string) ([]ApprovalRecord, error)
	CreateBatch(ctx context.Context, batch Batch, items []BatchItem) (Batch, []BatchItem, error)
	ListBatches(ctx context.Context, merchantID string, limit int) ([]Batch, error)
	GetBatch(ctx context.Context, merchantID, batchID string) (Batch, error)
	ListBatchItems(ctx context.Context, merchantID, batchID string) ([]BatchItem, error)
	UpdateBatchItem(ctx context.Context, batchItemID, payoutID, status, errorText string) error
	FinalizeBatch(ctx context.Context, batchID, status string, summary map[string]any) (Batch, error)
}
