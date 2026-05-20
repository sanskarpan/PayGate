package eventschema

import "time"

type Status string

const (
	StatusDraft      Status = "draft"
	StatusActive     Status = "active"
	StatusDeprecated Status = "deprecated"
	StatusRetired    Status = "retired"
)

type CheckType string

const (
	CheckTypeBackward CheckType = "backward"
	CheckTypeForward  CheckType = "forward"
)

type RolloutStatus string

const (
	RolloutStatusPlanned     RolloutStatus = "planned"
	RolloutStatusDualPublish RolloutStatus = "dual_publish"
	RolloutStatusCompleted   RolloutStatus = "completed"
	RolloutStatusRolledBack  RolloutStatus = "rolled_back"
)

type Schema struct {
	ID         string
	Subject    string
	EventType  string
	TopicName  string
	Owner      string
	ReviewLink string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Version struct {
	ID                   string
	Subject              string
	Version              string
	Status               Status
	Schema               Document
	SamplePayload        map[string]any
	ReviewLink           string
	CompatibilitySummary string
	CompatibilityDetails map[string]any
	ActivatedAt          *time.Time
	DeprecatedAt         *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type CompatibilityCheck struct {
	ID               string
	Subject          string
	CandidateVersion string
	BaselineVersion  string
	CheckType        CheckType
	Compatible       bool
	Summary          string
	Details          map[string]any
	CreatedAt        time.Time
}

type Rollout struct {
	ID              string
	Subject         string
	FromVersion     string
	ToVersion       string
	Status          RolloutStatus
	CutoverDeadline *time.Time
	Notes           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Consumers       []RolloutConsumer
}

type RolloutConsumer struct {
	ID                  string
	RolloutID           string
	ConsumerName        string
	AcknowledgedVersion string
	AcknowledgedAt      time.Time
	CreatedAt           time.Time
}

type DeprecatedVersionAlert struct {
	RolloutID           string
	Subject             string
	FromVersion         string
	ToVersion           string
	ConsumerName        string
	AcknowledgedVersion string
	CutoverDeadline     time.Time
}

type Document struct {
	Type       string                 `json:"type"`
	Properties map[string]FieldSchema `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
}

type FieldSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]FieldSchema `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
}
