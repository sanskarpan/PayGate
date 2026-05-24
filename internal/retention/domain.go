package retention

import "time"

type ArtifactClass string

const (
	ArtifactClassReportExport           ArtifactClass = "report_export"
	ArtifactClassWebhookDeliveryAttempt ArtifactClass = "webhook_delivery_attempt"
	ArtifactClassOnboardingDocument     ArtifactClass = "onboarding_document"
)

type PolicyAction string

const (
	PolicyActionRedactContent PolicyAction = "redact_content"
	PolicyActionRedactPayload PolicyAction = "redact_payload"
	PolicyActionRedactLocator PolicyAction = "redact_locator"
)

type Policy struct {
	ArtifactClass ArtifactClass `json:"artifact_class"`
	Action        PolicyAction  `json:"action"`
	RetainDays    int           `json:"retain_days"`
	Enabled       bool          `json:"enabled"`
	UpdatedBy     string        `json:"updated_by"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

type LegalHold struct {
	ID            string        `json:"id"`
	ArtifactClass ArtifactClass `json:"artifact_class"`
	MerchantID    string        `json:"merchant_id"`
	ArtifactID    string        `json:"artifact_id"`
	Reason        string        `json:"reason"`
	CreatedBy     string        `json:"created_by"`
	CreatedAt     time.Time     `json:"created_at"`
	ReleasedAt    *time.Time    `json:"released_at,omitempty"`
}

type Run struct {
	ID            string        `json:"id"`
	ArtifactClass ArtifactClass `json:"artifact_class"`
	Action        PolicyAction  `json:"action"`
	Status        string        `json:"status"`
	AffectedCount int           `json:"affected_count"`
	ErrorMessage  string        `json:"error_message"`
	ActorType     string        `json:"actor_type"`
	ActorID       string        `json:"actor_id"`
	StartedAt     time.Time     `json:"started_at"`
	CompletedAt   *time.Time    `json:"completed_at,omitempty"`
}

type CreateLegalHoldInput struct {
	ArtifactClass ArtifactClass
	MerchantID    string
	ArtifactID    string
	Reason        string
	CreatedBy     string
}

type RunInput struct {
	ArtifactClass ArtifactClass
	ActorType     string
	ActorID       string
}
