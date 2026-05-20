package eventschema

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSchemaNotFound        = errors.New("event schema not found")
	ErrSchemaVersionNotFound = errors.New("event schema version not found")
	ErrIncompatibleSchema    = errors.New("schema is not compatible with the current baseline")
	ErrNoActiveSchemaVersion = errors.New("no active schema version")
	ErrSchemaRolloutNotFound = errors.New("schema rollout not found")
	ErrInvalidSchemaDocument = errors.New("invalid schema document")
	ErrInvalidSchemaRollout  = errors.New("invalid schema rollout")
)

type CreateSchemaInput struct {
	Subject    string `json:"subject"`
	EventType  string `json:"event_type"`
	TopicName  string `json:"topic_name"`
	Owner      string `json:"owner"`
	ReviewLink string `json:"review_link"`
}

type CreateVersionInput struct {
	Subject       string
	Version       string
	Schema        Document
	SamplePayload map[string]any
	ReviewLink    string
}

type ActivateVersionInput struct {
	Subject string
	Version string
}

type CreateRolloutInput struct {
	Subject         string
	FromVersion     string
	ToVersion       string
	CutoverDeadline *time.Time
	Notes           string
}

type AckRolloutInput struct {
	RolloutID           string
	ConsumerName        string
	AcknowledgedVersion string
}

type CompareVersionsInput struct {
	Subject     string
	FromVersion string
	ToVersion   string
}

type Repository interface {
	CreateSchema(ctx context.Context, in CreateSchemaInput) (Schema, error)
	ListSchemas(ctx context.Context) ([]Schema, error)
	GetSchema(ctx context.Context, subject string) (Schema, error)
	CreateVersion(ctx context.Context, in CreateVersionInput, checks []CompatibilityCheck) (Version, error)
	ListVersions(ctx context.Context, subject string) ([]Version, error)
	GetVersion(ctx context.Context, subject, version string) (Version, error)
	GetActiveVersion(ctx context.Context, subject string) (Version, error)
	ActivateVersion(ctx context.Context, in ActivateVersionInput) (Version, error)
	ListChecks(ctx context.Context, subject string, limit int) ([]CompatibilityCheck, error)
	CreateRollout(ctx context.Context, in CreateRolloutInput) (Rollout, error)
	GetRollout(ctx context.Context, rolloutID string) (Rollout, error)
	GetActiveRollout(ctx context.Context, subject string) (Rollout, error)
	AckRollout(ctx context.Context, in AckRolloutInput) (RolloutConsumer, error)
	ListDeprecatedVersionAlerts(ctx context.Context) ([]DeprecatedVersionAlert, error)
}
