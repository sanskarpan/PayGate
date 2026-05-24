package saga

import "time"

type Status string

const (
	StatusPending      Status = "pending"
	StatusRunning      Status = "running"
	StatusWaiting      Status = "waiting"
	StatusCompensating Status = "compensating"
	StatusCompleted    Status = "completed"
	StatusFailed       Status = "failed"
	StatusAborted      Status = "aborted"
)

type StepKind string

const (
	StepKindCommand      StepKind = "command"
	StepKindWait         StepKind = "wait"
	StepKindCompensation StepKind = "compensation"
)

type StepStatus string

const (
	StepStatusPending     StepStatus = "pending"
	StepStatusInProgress  StepStatus = "in_progress"
	StepStatusCompleted   StepStatus = "completed"
	StepStatusFailed      StepStatus = "failed"
	StepStatusCompensated StepStatus = "compensated"
	StepStatusCancelled   StepStatus = "cancelled"
)

type Instance struct {
	ID               string
	MerchantID       string
	SagaType         string
	Status           Status
	CorrelationID    string
	CausationID      string
	InputPayload     map[string]any
	ContextPayload   map[string]any
	CurrentStepIndex int
	FailureCode      string
	FailureReason    string
	LeasedBy         string
	LastLeasedAt     *time.Time
	ReplayCount      int
	DeadlineAt       *time.Time
	TimeoutAt        *time.Time
	StartedAt        time.Time
	CompletedAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Steps            []Step
}

type Step struct {
	ID            string
	SagaID        string
	MerchantID    string
	StepIndex     int
	StepName      string
	StepKind      StepKind
	Status        StepStatus
	CommandName   string
	CommandID     string
	ReplyTopic    string
	InputPayload  map[string]any
	OutputPayload map[string]any
	ErrorCode     string
	ErrorMessage  string
	NextRetryAt   time.Time
	LeasedBy      string
	LeasedAt      *time.Time
	CompletedAt   *time.Time
	AttemptCount  int
	MaxAttempts   int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Command struct {
	SagaID          string
	StepID          string
	MerchantID      string
	CommandName     string
	CommandID       string
	CorrelationID   string
	CausationID     string
	ReplyTopic      string
	DispatchAttempt int
	MaxAttempts     int
	RequestedAt     time.Time
	InputPayload    map[string]any
}

type DeadLetterType string

const (
	DeadLetterTypeCommandFailed      DeadLetterType = "command_failed"
	DeadLetterTypeTimeout            DeadLetterType = "timeout"
	DeadLetterTypeOverride           DeadLetterType = "override"
	DeadLetterTypeCompensationFailed DeadLetterType = "compensation_failed"
)

type DeadLetter struct {
	ID             string
	SagaID         string
	StepID         string
	MerchantID     string
	DeadLetterType DeadLetterType
	CommandName    string
	CommandID      string
	ErrorCode      string
	ErrorMessage   string
	Payload        map[string]any
	CreatedAt      time.Time
}

type OperatorAction struct {
	ID         string
	SagaID     string
	MerchantID string
	Action     string
	ActorType  string
	ActorID    string
	Reason     string
	Payload    map[string]any
	CreatedAt  time.Time
}

type DispatchStatus string

const (
	DispatchStatusDispatched DispatchStatus = "dispatched"
	DispatchStatusAcked      DispatchStatus = "acked"
	DispatchStatusNacked     DispatchStatus = "nacked"
)

type CommandDispatch struct {
	ID                  string
	SagaID              string
	StepID              string
	MerchantID          string
	CommandName         string
	CommandID           string
	DispatchAttempt     int
	Status              DispatchStatus
	LeasedBy            string
	LeasedAt            time.Time
	AckedAt             *time.Time
	NackedAt            *time.Time
	RetryBackoffSeconds int
	ErrorCode           string
	ErrorMessage        string
	ErrorClassification string
	InputPayload        map[string]any
	OutputPayload       map[string]any
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type OverrideAction string

const (
	OverrideActionAbort         OverrideAction = "abort"
	OverrideActionForceComplete OverrideAction = "force_complete"
)

func (s Step) Command() Command {
	return Command{
		SagaID:       s.SagaID,
		StepID:       s.ID,
		MerchantID:   s.MerchantID,
		CommandName:  s.CommandName,
		CommandID:    s.CommandID,
		InputPayload: clonePayload(s.InputPayload),
	}
}

func clonePayload(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
