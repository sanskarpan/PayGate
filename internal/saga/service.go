package saga

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"
)

type CommandHandler func(context.Context, Command) (map[string]any, error)
type CompensationHandler func(context.Context, Instance) error

type Service struct {
	repo                 Repository
	logger               *slog.Logger
	handlers             map[string]CommandHandler
	compensationHandlers map[string]CompensationHandler
	handlersMu           sync.RWMutex
	leaseTTL             time.Duration
}

func NewService(repo Repository, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:                 repo,
		logger:               logger,
		handlers:             map[string]CommandHandler{},
		compensationHandlers: map[string]CompensationHandler{},
		leaseTTL:             30 * time.Second,
	}
}

func (s *Service) RegisterHandler(commandName string, handler CommandHandler) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.handlers[commandName] = handler
}

func (s *Service) RegisterCompensationHandler(sagaType string, handler CompensationHandler) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.compensationHandlers[sagaType] = handler
}

func (s *Service) SetLeaseTTLForTest(ttl time.Duration) {
	if ttl > 0 {
		s.leaseTTL = ttl
	}
}

func (s *Service) StartCommandSaga(ctx context.Context, in CreateCommandSagaInput) (Instance, error) {
	return s.repo.CreateCommandSaga(ctx, in)
}

func (s *Service) Get(ctx context.Context, merchantID, sagaID string) (Instance, error) {
	return s.repo.Get(ctx, merchantID, sagaID)
}

func (s *Service) List(ctx context.Context, merchantID string, limit int) ([]Instance, error) {
	return s.repo.List(ctx, merchantID, limit)
}

func (s *Service) Replay(ctx context.Context, in ReplayInput) (Instance, error) {
	return s.repo.Replay(ctx, in)
}

func (s *Service) Override(ctx context.Context, in OverrideInput) (Instance, error) {
	current, err := s.repo.Get(ctx, in.MerchantID, in.SagaID)
	if err != nil {
		return Instance{}, err
	}
	switch in.Action {
	case OverrideActionAbort:
		if err := s.repo.RecordDeadLetter(ctx, DeadLetterInput{
			SagaID:         current.ID,
			MerchantID:     current.MerchantID,
			DeadLetterType: DeadLetterTypeOverride,
			ErrorCode:      "OPERATOR_ABORT",
			ErrorMessage:   in.Reason,
			Payload:        map[string]any{"status_before": current.Status, "action": in.Action},
		}); err != nil {
			return Instance{}, err
		}
		return s.abortWithCompensation(ctx, current, "OPERATOR_ABORT", in.Reason)
	case OverrideActionForceComplete:
		return s.repo.ForceComplete(ctx, in)
	default:
		return Instance{}, ErrInvalidOverride
	}
}

func (s *Service) ListDeadLetters(ctx context.Context, merchantID, sagaID string, limit int) ([]DeadLetter, error) {
	return s.repo.ListDeadLetters(ctx, merchantID, sagaID, limit)
}

func (s *Service) ListOperatorActions(ctx context.Context, merchantID, sagaID string, limit int) ([]OperatorAction, error) {
	return s.repo.ListOperatorActions(ctx, merchantID, sagaID, limit)
}

func (s *Service) ListDispatches(ctx context.Context, merchantID, sagaID string, limit int) ([]CommandDispatch, error) {
	return s.repo.ListDispatches(ctx, merchantID, sagaID, limit)
}

func (s *Service) ProcessNextTimeout(ctx context.Context, leaseOwner string) (bool, error) {
	instance, ok, err := s.repo.LeaseTimedOutSaga(ctx, leaseOwner, s.leaseTTL)
	if err != nil || !ok {
		return ok, err
	}
	if err := s.repo.RecordDeadLetter(ctx, DeadLetterInput{
		SagaID:         instance.ID,
		MerchantID:     instance.MerchantID,
		DeadLetterType: DeadLetterTypeTimeout,
		ErrorCode:      "SAGA_TIMEOUT",
		ErrorMessage:   "saga timed out before reaching a terminal state",
		Payload: map[string]any{
			"saga_type": instance.SagaType,
			"status":    instance.Status,
		},
	}); err != nil {
		return true, err
	}
	if _, err := s.abortWithCompensation(ctx, instance, "SAGA_TIMEOUT", "saga timed out before reaching a terminal state"); err != nil {
		return true, err
	}
	return true, nil
}

func (s *Service) RunNext(ctx context.Context, leaseOwner string) (bool, error) {
	step, ok, err := s.repo.LeaseNextCommandStep(ctx, leaseOwner, s.leaseTTL)
	if err != nil || !ok {
		return ok, err
	}
	if _, err := s.repo.RecordDispatch(ctx, RecordDispatchInput{
		SagaID:          step.SagaID,
		StepID:          step.ID,
		MerchantID:      step.MerchantID,
		CommandName:     step.CommandName,
		CommandID:       step.CommandID,
		DispatchAttempt: step.AttemptCount,
		LeasedBy:        leaseOwner,
		InputPayload:    step.InputPayload,
	}); err != nil {
		return true, err
	}

	s.handlersMu.RLock()
	handler, found := s.handlers[step.CommandName]
	s.handlersMu.RUnlock()
	if !found {
		if err := s.repo.NackDispatch(ctx, NackDispatchInput{
			StepID:              step.ID,
			CommandID:           step.CommandID,
			ErrorCode:           "NO_HANDLER",
			ErrorMessage:        fmt.Sprintf("no command handler registered for %s", step.CommandName),
			ErrorClassification: "configuration",
		}); err != nil {
			return true, err
		}
		instance, failErr := s.repo.FailStep(ctx, FailStepInput{
			StepID:       step.ID,
			ErrorCode:    "NO_HANDLER",
			ErrorMessage: fmt.Sprintf("no command handler registered for %s", step.CommandName),
			Terminal:     true,
		})
		if failErr != nil {
			return true, failErr
		}
		return true, s.handleTerminalFailure(ctx, instance, step, "NO_HANDLER", fmt.Sprintf("no command handler registered for %s", step.CommandName))
	}

	result, err := handler(ctx, step.Command())
	if err != nil {
		terminal := step.AttemptCount >= step.MaxAttempts
		classification := "transient"
		if terminal {
			classification = "poison"
		}
		if err := s.repo.NackDispatch(ctx, NackDispatchInput{
			StepID:              step.ID,
			CommandID:           step.CommandID,
			ErrorCode:           "COMMAND_FAILED",
			ErrorMessage:        err.Error(),
			ErrorClassification: classification,
			RetryBackoff:        retryBackoff(step.AttemptCount),
		}); err != nil {
			return true, err
		}
		instance, failErr := s.repo.FailStep(ctx, FailStepInput{
			StepID:       step.ID,
			ErrorCode:    "COMMAND_FAILED",
			ErrorMessage: err.Error(),
			RetryBackoff: retryBackoff(step.AttemptCount),
			Terminal:     terminal,
		})
		if failErr != nil {
			return true, failErr
		}
		if terminal {
			return true, s.handleTerminalFailure(ctx, instance, step, "COMMAND_FAILED", err.Error())
		}
		return true, nil
	}

	dedupedResult, _, err := s.repo.RecordProcessedCommand(ctx, step.CommandName, step.CommandID, result)
	if err != nil {
		_ = s.repo.NackDispatch(ctx, NackDispatchInput{
			StepID:              step.ID,
			CommandID:           step.CommandID,
			ErrorCode:           "COMMAND_DEDUP_STORE_FAILED",
			ErrorMessage:        err.Error(),
			ErrorClassification: "infrastructure",
		})
		return true, err
	}
	if err := s.repo.AckDispatch(ctx, AckDispatchInput{
		StepID:        step.ID,
		CommandID:     step.CommandID,
		CommandOutput: dedupedResult,
	}); err != nil {
		return true, err
	}
	_, err = s.repo.CompleteStep(ctx, CompleteStepInput{
		StepID:        step.ID,
		CommandOutput: dedupedResult,
	})
	return true, err
}

func (s *Service) abortWithCompensation(ctx context.Context, current Instance, code, reason string) (Instance, error) {
	handler := s.compensationHandler(current.SagaType)
	if handler == nil {
		return s.repo.Abort(ctx, OverrideInput{
			MerchantID: current.MerchantID,
			SagaID:     current.ID,
			Action:     OverrideActionAbort,
			ActorType:  "system",
			Reason:     reason,
		})
	}
	if _, err := s.repo.MarkCompensating(ctx, current.MerchantID, current.ID, code, reason); err != nil {
		return Instance{}, err
	}
	updated, err := s.repo.Get(ctx, current.MerchantID, current.ID)
	if err != nil {
		return Instance{}, err
	}
	if err := handler(ctx, updated); err != nil {
		if recordErr := s.repo.RecordDeadLetter(ctx, DeadLetterInput{
			SagaID:         current.ID,
			MerchantID:     current.MerchantID,
			DeadLetterType: DeadLetterTypeCompensationFailed,
			ErrorCode:      "COMPENSATION_FAILED",
			ErrorMessage:   err.Error(),
			Payload:        map[string]any{"saga_type": current.SagaType},
		}); recordErr != nil {
			return Instance{}, recordErr
		}
		return s.repo.FailSaga(ctx, FailSagaInput{
			MerchantID:    current.MerchantID,
			SagaID:        current.ID,
			FailureCode:   "COMPENSATION_FAILED",
			FailureReason: err.Error(),
		})
	}
	return s.repo.Abort(ctx, OverrideInput{
		MerchantID: current.MerchantID,
		SagaID:     current.ID,
		Action:     OverrideActionAbort,
		ActorType:  "system",
		Reason:     reason,
	})
}

func (s *Service) handleTerminalFailure(ctx context.Context, current Instance, step Step, code, message string) error {
	if err := s.repo.RecordDeadLetter(ctx, DeadLetterInput{
		SagaID:         current.ID,
		StepID:         step.ID,
		MerchantID:     current.MerchantID,
		DeadLetterType: DeadLetterTypeCommandFailed,
		CommandName:    step.CommandName,
		CommandID:      step.CommandID,
		ErrorCode:      code,
		ErrorMessage:   message,
		Payload: map[string]any{
			"step_name":    step.StepName,
			"step_index":   step.StepIndex,
			"attempts":     step.AttemptCount,
			"max_attempts": step.MaxAttempts,
		},
	}); err != nil {
		return err
	}
	_, err := s.abortWithCompensation(ctx, current, code, message)
	return err
}

func (s *Service) compensationHandler(sagaType string) CompensationHandler {
	s.handlersMu.RLock()
	defer s.handlersMu.RUnlock()
	return s.compensationHandlers[sagaType]
}

func retryBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return time.Second
	}
	power := math.Min(float64(attempt-1), 5)
	return time.Duration(math.Pow(2, power)) * time.Second
}
