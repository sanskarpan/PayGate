package saga

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSagaNotFound      = errors.New("saga not found")
	ErrSagaNotReplayable = errors.New("saga is not replayable")
	ErrInvalidOverride   = errors.New("invalid saga override")
)

type CreateCommandSagaInput struct {
	MerchantID     string
	SagaType       string
	CorrelationID  string
	CausationID    string
	InputPayload   map[string]any
	ContextPayload map[string]any
	DeadlineAt     *time.Time
	TimeoutAt      *time.Time
	InitialStep    CreateStepInput
}

type CreateStepInput struct {
	StepName     string
	StepKind     StepKind
	CommandName  string
	ReplyTopic   string
	InputPayload map[string]any
	MaxAttempts  int
}

type CompleteStepInput struct {
	StepID        string
	CommandOutput map[string]any
}

type FailStepInput struct {
	StepID       string
	ErrorCode    string
	ErrorMessage string
	RetryBackoff time.Duration
	Terminal     bool
}

type ReplayInput struct {
	MerchantID string
	SagaID     string
	Force      bool
	DryRun     bool
	ActorType  string
	ActorID    string
	Reason     string
}

type DeadLetterInput struct {
	SagaID         string
	StepID         string
	MerchantID     string
	DeadLetterType DeadLetterType
	CommandName    string
	CommandID      string
	ErrorCode      string
	ErrorMessage   string
	Payload        map[string]any
}

type OperatorActionInput struct {
	MerchantID string
	SagaID     string
	Action     string
	ActorType  string
	ActorID    string
	Reason     string
	Payload    map[string]any
}

type RecordDispatchInput struct {
	SagaID          string
	StepID          string
	MerchantID      string
	CommandName     string
	CommandID       string
	DispatchAttempt int
	LeasedBy        string
	InputPayload    map[string]any
}

type AckDispatchInput struct {
	StepID        string
	CommandID     string
	CommandOutput map[string]any
}

type NackDispatchInput struct {
	StepID              string
	CommandID           string
	ErrorCode           string
	ErrorMessage        string
	ErrorClassification string
	RetryBackoff        time.Duration
}

type FailSagaInput struct {
	MerchantID    string
	SagaID        string
	FailureCode   string
	FailureReason string
}

type OverrideInput struct {
	MerchantID string
	SagaID     string
	Action     OverrideAction
	ActorType  string
	ActorID    string
	Reason     string
}

type Repository interface {
	CreateCommandSaga(ctx context.Context, in CreateCommandSagaInput) (Instance, error)
	Get(ctx context.Context, merchantID, sagaID string) (Instance, error)
	List(ctx context.Context, merchantID string, limit int) ([]Instance, error)
	LeaseNextCommandStep(ctx context.Context, leaseOwner string, leaseTTL time.Duration) (Step, bool, error)
	LeaseTimedOutSaga(ctx context.Context, leaseOwner string, leaseTTL time.Duration) (Instance, bool, error)
	CompleteStep(ctx context.Context, in CompleteStepInput) (Instance, error)
	FailStep(ctx context.Context, in FailStepInput) (Instance, error)
	FailSaga(ctx context.Context, in FailSagaInput) (Instance, error)
	MarkCompensating(ctx context.Context, merchantID, sagaID, failureCode, failureReason string) (Instance, error)
	Abort(ctx context.Context, in OverrideInput) (Instance, error)
	ForceComplete(ctx context.Context, in OverrideInput) (Instance, error)
	Replay(ctx context.Context, in ReplayInput) (Instance, error)
	RecordDeadLetter(ctx context.Context, in DeadLetterInput) error
	RecordOperatorAction(ctx context.Context, in OperatorActionInput) error
	ListDeadLetters(ctx context.Context, merchantID, sagaID string, limit int) ([]DeadLetter, error)
	ListOperatorActions(ctx context.Context, merchantID, sagaID string, limit int) ([]OperatorAction, error)
	RecordDispatch(ctx context.Context, in RecordDispatchInput) (CommandDispatch, error)
	AckDispatch(ctx context.Context, in AckDispatchInput) error
	NackDispatch(ctx context.Context, in NackDispatchInput) error
	ListDispatches(ctx context.Context, merchantID, sagaID string, limit int) ([]CommandDispatch, error)
	RecordProcessedCommand(ctx context.Context, consumerName, commandID string, result map[string]any) (map[string]any, bool, error)
}
