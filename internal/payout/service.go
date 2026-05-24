package payout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/middleware"
	"github.com/sanskarpan/PayGate/internal/ledger"
	"github.com/sanskarpan/PayGate/internal/saga"
)

// Service orchestrates payout use-cases.
type Service struct {
	repo             Repository
	logger           *slog.Logger
	ledgerSvc        *ledger.Service
	sagaSvc          *saga.Service
	transferExecutor func(context.Context, string, string, string) (map[string]any, error)
	railSecret       string
}

// NewService creates a new Service.
func NewService(repo Repository, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, logger: logger}
}

func (s *Service) SetRailCallbackSecret(secret string) {
	s.railSecret = secret
}

func (s *Service) SetLedgerService(ledgerSvc *ledger.Service) {
	s.ledgerSvc = ledgerSvc
}

func (s *Service) EnableSagaOrchestration(svc *saga.Service) {
	s.sagaSvc = svc
}

func (s *Service) RegisterSagaHandlers(svc *saga.Service) {
	svc.RegisterHandler("payout.complete_transfer", func(ctx context.Context, cmd saga.Command) (map[string]any, error) {
		return s.executeTransfer(ctx, cmd.MerchantID, stringValue(cmd.InputPayload["payout_id"]), cmd.CommandID)
	})
	svc.RegisterCompensationHandler("payout_execution", func(ctx context.Context, inst saga.Instance) error {
		payoutID := stringValue(inst.ContextPayload["payout_id"])
		if payoutID == "" {
			payoutID = stringValue(inst.InputPayload["payout_id"])
		}
		if payoutID == "" {
			return nil
		}
		current, err := s.repo.GetByID(ctx, inst.MerchantID, payoutID)
		if err != nil {
			if errors.Is(err, ErrPayoutNotFound) {
				return nil
			}
			return err
		}
		switch current.Status {
		case StateCompleted, StateFailed, StateReturned, StateReversed, StateCancelled:
			return nil
		default:
			_, err := s.repo.Fail(ctx, inst.MerchantID, payoutID, "saga compensation applied: "+inst.FailureReason)
			return err
		}
	})
}

func (s *Service) SetTransferExecutorForTest(fn func(context.Context, string, string, string) (map[string]any, error)) {
	s.transferExecutor = fn
}

// InitiatePayout creates a payout for the given settlement if one does not already exist,
// transitions it to processing, and launches execution.
func (s *Service) InitiatePayout(ctx context.Context, merchantID, settlementID string) (Payout, error) {
	p, err := s.repo.GetBySettlementID(ctx, merchantID, settlementID)
	if err != nil {
		if !errors.Is(err, ErrPayoutNotFound) {
			return Payout{}, err
		}
		return Payout{}, ErrPayoutNotFound
	}

	p, err = s.repo.Initiate(ctx, merchantID, p.ID)
	if err != nil {
		return Payout{}, err
	}
	if err := s.launchPayoutExecution(ctx, &p); err != nil {
		return Payout{}, err
	}
	return p, nil
}

// InitiatePayoutForSettlement creates a payout for the given settlement using the provided
// amount and currency, then transitions it to processing and launches execution.
func (s *Service) InitiatePayoutForSettlement(ctx context.Context, merchantID, settlementID, beneficiaryID string, amount int64, currency string, approvalThresholdAmount int64, batchID string) (Payout, error) {
	if s.ledgerSvc != nil {
		if err := s.ledgerSvc.CanReserveForPayout(ctx, merchantID, currency, amount); err != nil {
			return Payout{}, err
		}
	}
	beneficiary, err := s.repo.GetBeneficiary(ctx, merchantID, beneficiaryID)
	if err != nil {
		return Payout{}, err
	}
	if beneficiary.Status != BeneficiaryStatusApproved {
		return Payout{}, ErrBeneficiaryNotApproved
	}
	existing, err := s.repo.GetBySettlementID(ctx, merchantID, settlementID)
	if err != nil && !errors.Is(err, ErrPayoutNotFound) {
		return Payout{}, err
	}

	var p Payout
	if errors.Is(err, ErrPayoutNotFound) {
		approvalStatus := ApprovalStatusNotRequired
		if approvalThresholdAmount > 0 && amount >= approvalThresholdAmount {
			approvalStatus = ApprovalStatusPending
		}
		p, err = s.repo.CreateForSettlement(ctx, merchantID, settlementID, beneficiaryID, amount, currency, approvalStatus, batchID)
		if err != nil {
			return Payout{}, fmt.Errorf("create payout: %w", err)
		}
	} else {
		p = existing
	}
	if p.ApprovalStatus == ApprovalStatusPending {
		return p, nil
	}

	p, err = s.repo.Initiate(ctx, merchantID, p.ID)
	if err != nil {
		return Payout{}, fmt.Errorf("initiate payout: %w", err)
	}
	if err := s.launchPayoutExecution(ctx, &p); err != nil {
		return Payout{}, err
	}
	return p, nil
}

// Get returns a payout by ID scoped to the merchant.
func (s *Service) Get(ctx context.Context, merchantID, id string) (Payout, error) {
	return s.repo.GetByID(ctx, merchantID, id)
}

// List returns payouts for a merchant, most recent first up to limit.
func (s *Service) List(ctx context.Context, merchantID string, limit int) ([]Payout, error) {
	return s.repo.List(ctx, merchantID, limit)
}

// GetBySettlement returns the payout associated with a settlement.
func (s *Service) GetBySettlement(ctx context.Context, merchantID, settlementID string) (Payout, error) {
	return s.repo.GetBySettlementID(ctx, merchantID, settlementID)
}

func (s *Service) ListEvents(ctx context.Context, merchantID, payoutID string, limit int) ([]TimelineEvent, error) {
	return s.repo.ListEvents(ctx, merchantID, payoutID, limit)
}

func (s *Service) Cancel(ctx context.Context, merchantID, payoutID, reason string) (Payout, error) {
	current, err := s.repo.Cancel(ctx, merchantID, payoutID, reason)
	if err != nil {
		return Payout{}, err
	}
	if s.sagaSvc != nil && current.SagaID != "" {
		if _, overrideErr := s.sagaSvc.Override(ctx, saga.OverrideInput{
			MerchantID: current.MerchantID,
			SagaID:     current.SagaID,
			Action:     saga.OverrideActionAbort,
			Reason:     reason,
		}); overrideErr != nil {
			s.logger.Warn("payout cancel saga override failed", "payout_id", payoutID, "saga_id", current.SagaID, "error", overrideErr)
		}
	}
	return current, nil
}

func (s *Service) CreateBeneficiary(ctx context.Context, beneficiary Beneficiary, actor, actorScope string) (Beneficiary, error) {
	if err := beneficiary.Validate(); err != nil {
		return Beneficiary{}, err
	}
	return s.repo.CreateBeneficiary(ctx, beneficiary, actor, actorScope)
}

func (s *Service) ListBeneficiaries(ctx context.Context, merchantID string) ([]Beneficiary, error) {
	return s.repo.ListBeneficiaries(ctx, merchantID)
}

func (s *Service) GetBeneficiary(ctx context.Context, merchantID, beneficiaryID string) (Beneficiary, error) {
	return s.repo.GetBeneficiary(ctx, merchantID, beneficiaryID)
}

func (s *Service) VerifyBeneficiary(ctx context.Context, merchantID, beneficiaryID string) (Beneficiary, BeneficiaryVerification, error) {
	evidence := map[string]any{"method": "simulated_penny_drop", "result": "passed"}
	return s.repo.VerifyBeneficiary(ctx, merchantID, beneficiaryID, evidence)
}

func (s *Service) ApproveBeneficiary(ctx context.Context, merchantID, beneficiaryID, notes, actor, actorScope string) (Beneficiary, error) {
	return s.repo.ApproveBeneficiary(ctx, merchantID, beneficiaryID, notes, actor, actorScope)
}

func (s *Service) ApprovePayout(ctx context.Context, merchantID, payoutID, actor, actorScope, notes string) (Payout, error) {
	payoutOut, err := s.repo.RecordApproval(ctx, merchantID, payoutID, actor, actorScope, "approved", notes)
	if err != nil {
		return Payout{}, err
	}
	if payoutOut.ApprovalStatus == ApprovalStatusApproved && payoutOut.Status == StatePending {
		payoutOut, err = s.repo.Initiate(ctx, merchantID, payoutID)
		if err != nil {
			return Payout{}, err
		}
		if err := s.launchPayoutExecution(ctx, &payoutOut); err != nil {
			return Payout{}, err
		}
	}
	return payoutOut, nil
}

func (s *Service) RejectPayout(ctx context.Context, merchantID, payoutID, actor, actorScope, notes string) (Payout, error) {
	return s.repo.RecordApproval(ctx, merchantID, payoutID, actor, actorScope, "rejected", notes)
}

func (s *Service) ListApprovals(ctx context.Context, merchantID, payoutID string) ([]ApprovalRecord, error) {
	return s.repo.ListApprovals(ctx, merchantID, payoutID)
}

func (s *Service) CreateBatch(ctx context.Context, merchantID, idempotencyKey string, dryRun bool, items []BatchItem, amountThreshold int64) (Batch, []BatchItem, error) {
	batch, persistedItems, err := s.repo.CreateBatch(ctx, Batch{
		ID:             "",
		MerchantID:     merchantID,
		DryRun:         dryRun,
		Status:         "created",
		IdempotencyKey: idempotencyKey,
		Summary:        map[string]any{},
	}, items)
	if err != nil {
		return Batch{}, nil, err
	}
	if dryRun {
		return batch, persistedItems, nil
	}
	successes := 0
	failures := 0
	for i := range persistedItems {
		payoutOut, payoutErr := s.InitiatePayoutForSettlement(ctx, merchantID, persistedItems[i].SettlementID, persistedItems[i].BeneficiaryID, persistedItems[i].Amount, persistedItems[i].Currency, amountThreshold, batch.ID)
		if payoutErr != nil {
			failures++
			_ = s.repo.UpdateBatchItem(ctx, persistedItems[i].ID, "", "failed", payoutErr.Error())
			persistedItems[i].Status = "failed"
			persistedItems[i].ErrorText = payoutErr.Error()
			continue
		}
		successes++
		_ = s.repo.UpdateBatchItem(ctx, persistedItems[i].ID, payoutOut.ID, "created", "")
		persistedItems[i].PayoutID = payoutOut.ID
		persistedItems[i].Status = "created"
	}
	status := "completed"
	if failures > 0 {
		status = "partial_failed"
	}
	batch, err = s.repo.FinalizeBatch(ctx, batch.ID, status, map[string]any{"successes": successes, "failures": failures})
	if err != nil {
		return Batch{}, nil, err
	}
	return batch, persistedItems, nil
}

func (s *Service) UpsertSimulatorScenario(ctx context.Context, merchantID, settlementID string, scenario SimulatorScenario) (SimulatorScenario, error) {
	if err := validateSimulatorScenario(scenario); err != nil {
		return SimulatorScenario{}, err
	}
	return s.repo.UpsertSimulatorScenario(ctx, merchantID, settlementID, scenario)
}

func (s *Service) GetSimulatorScenario(ctx context.Context, merchantID, settlementID string) (SimulatorScenario, error) {
	scenario, err := s.repo.GetSimulatorScenario(ctx, merchantID, settlementID)
	if errors.Is(err, ErrSimulatorScenarioNotFound) {
		scenario = defaultSimulatorScenario()
		scenario.MerchantID = merchantID
		scenario.SettlementID = settlementID
		return scenario, nil
	}
	return scenario, err
}

func (s *Service) ApplyRailCallback(ctx context.Context, callback RailCallback, signature string) (Payout, bool, error) {
	return s.repo.ApplyRailCallback(ctx, callback, signature)
}

func (s *Service) launchPayoutExecution(ctx context.Context, p *Payout) error {
	if p.Status == StateCompleted || p.Status == StateFailed || p.Status == StateCancelled {
		return nil
	}
	if s.sagaSvc == nil {
		go s.simulateBankTransfer(context.WithoutCancel(ctx), p.MerchantID, p.ID)
		return nil
	}
	if p.SagaID != "" {
		return nil
	}

	correlationID, _ := middleware.RequestIDFromContext(ctx)
	instance, err := s.sagaSvc.StartCommandSaga(ctx, saga.CreateCommandSagaInput{
		MerchantID:    p.MerchantID,
		SagaType:      "payout_execution",
		CorrelationID: correlationID,
		InputPayload: map[string]any{
			"payout_id":     p.ID,
			"settlement_id": p.SettlementID,
			"amount":        p.Amount,
			"currency":      p.Currency,
		},
		ContextPayload: map[string]any{
			"payout_id": p.ID,
		},
		InitialStep: saga.CreateStepInput{
			StepName:    "complete_payout_transfer",
			StepKind:    saga.StepKindCommand,
			CommandName: "payout.complete_transfer",
			InputPayload: map[string]any{
				"payout_id": p.ID,
			},
			MaxAttempts: 3,
		},
	})
	if err != nil {
		if _, failErr := s.repo.Fail(ctx, p.MerchantID, p.ID, "failed to start payout saga: "+err.Error()); failErr != nil {
			s.logger.Error("payout saga start failure handling failed", "payout_id", p.ID, "error", failErr)
		}
		return fmt.Errorf("start payout saga: %w", err)
	}
	updated, err := s.repo.AttachSaga(ctx, p.MerchantID, p.ID, instance.ID)
	if err != nil {
		if _, failErr := s.repo.Fail(ctx, p.MerchantID, p.ID, "failed to attach payout saga: "+err.Error()); failErr != nil {
			s.logger.Error("payout saga attach failure handling failed", "payout_id", p.ID, "error", failErr)
		}
		return fmt.Errorf("attach payout saga: %w", err)
	}
	*p = updated
	return nil
}

// simulateBankTransfer sleeps 2s then completes (or fails) the payout.
func (s *Service) simulateBankTransfer(ctx context.Context, merchantID, payoutID string) {
	if _, err := s.executeTransfer(ctx, merchantID, payoutID, ""); err != nil {
		s.logger.Error("simulate bank transfer failed", "payout_id", payoutID, "merchant_id", merchantID, "error", err)
		if _, ferr := s.repo.Fail(ctx, merchantID, payoutID, err.Error()); ferr != nil {
			s.logger.Error("simulate bank transfer: fail also failed", "payout_id", payoutID, "error", ferr)
		}
	}
}

func (s *Service) executeTransfer(ctx context.Context, merchantID, payoutID, commandID string) (map[string]any, error) {
	current, err := s.repo.GetByID(ctx, merchantID, payoutID)
	if err == nil {
		switch current.Status {
		case StateCompleted, StateFailed, StateReturned, StateReversed, StateCancelled:
			return map[string]any{
				"payout_id":      current.ID,
				"bank_reference": current.BankReference,
				"status":         current.Status,
			}, nil
		}
	}

	if s.transferExecutor != nil {
		return s.transferExecutor(ctx, merchantID, payoutID, commandID)
	}

	baseID := commandID
	if baseID == "" {
		baseID = fmt.Sprintf("rail_%d", time.Now().UnixNano())
	}
	scenario, shouldFail, err := s.repo.GetSimulatorScenarioForPayout(ctx, merchantID, payoutID)
	if err != nil {
		return nil, err
	}
	if shouldFail {
		return nil, errors.New("simulated payout rail transient failure")
	}
	if len(scenario.Steps) == 0 {
		scenario = defaultSimulatorScenario()
	}

	var payoutOut Payout
	for i, step := range scenario.Steps {
		eventID := fmt.Sprintf("%s_%s_%d", baseID, step.Status, i)
		occurredAt := time.Now().UTC()
		if step.DelayMilliseconds > 0 {
			time.Sleep(time.Duration(step.DelayMilliseconds) * time.Millisecond)
		}
		callbackOut, _, err := s.repo.ApplyRailCallback(ctx, RailCallback{
			EventID:       eventID,
			PayoutID:      payoutID,
			MerchantID:    merchantID,
			Status:        step.Status,
			RailReference: defaultRailReference(step, step.Status),
			Reason:        step.Reason,
			OccurredAt:    occurredAt,
		}, "simulated")
		if err != nil && !errors.Is(err, ErrInvalidTransition) {
			return nil, err
		}
		if err == nil {
			payoutOut = callbackOut
		}
		for dup := 0; dup < step.DuplicateCount; dup++ {
			callbackOut, _, err = s.repo.ApplyRailCallback(ctx, RailCallback{
				EventID:       eventID,
				PayoutID:      payoutID,
				MerchantID:    merchantID,
				Status:        step.Status,
				RailReference: defaultRailReference(step, step.Status),
				Reason:        step.Reason,
				OccurredAt:    occurredAt,
			}, "simulated")
			if err != nil && !errors.Is(err, ErrInvalidTransition) {
				return nil, err
			}
			if err == nil {
				payoutOut = callbackOut
			}
		}
	}
	if payoutOut.ID == "" {
		payoutOut, err = s.repo.GetByID(ctx, merchantID, payoutID)
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"payout_id":      payoutOut.ID,
		"bank_reference": payoutOut.BankReference,
		"status":         payoutOut.Status,
	}, nil
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func validateSimulatorScenario(scenario SimulatorScenario) error {
	for _, step := range scenario.Steps {
		switch step.Status {
		case RailStatusProcessing, RailStatusCompleted, RailStatusFailed, RailStatusReturned, RailStatusReversed:
		default:
			return fmt.Errorf("%w: unsupported step status %q", ErrInvalidSimulatorScenario, step.Status)
		}
		if step.DelayMilliseconds < 0 || step.DuplicateCount < 0 {
			return fmt.Errorf("%w: negative delay or duplicate count", ErrInvalidSimulatorScenario)
		}
	}
	if scenario.TransientFailuresRemaining < 0 {
		return fmt.Errorf("%w: transient_failures_remaining must be >= 0", ErrInvalidSimulatorScenario)
	}
	return nil
}

func defaultSimulatorScenario() SimulatorScenario {
	return SimulatorScenario{
		Steps: []SimulatorScenarioStep{
			{Status: RailStatusProcessing},
			{Status: RailStatusCompleted, DelayMilliseconds: 2000},
		},
	}
}

func defaultRailReference(step SimulatorScenarioStep, status RailCallbackStatus) string {
	if step.RailReference != "" {
		return step.RailReference
	}
	if status == RailStatusCompleted {
		return fmt.Sprintf("BNK_%d", time.Now().UnixNano())
	}
	return ""
}
