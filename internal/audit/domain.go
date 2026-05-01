package audit

import "time"

// ActorType classifies the identity that performed the action.
type ActorType string

const (
	ActorTypeDashboardUser ActorType = "dashboard_user"
	ActorTypeAPIKey        ActorType = "api_key"
	ActorTypeSystem        ActorType = "system"
)

// Log is an append-only record of a state-changing action.
type Log struct {
	ID            string
	MerchantID    string
	ActorID       string
	ActorEmail    string
	ActorType     ActorType
	Action        string
	ResourceType  string
	ResourceID    string
	Changes       map[string]any
	IPAddress     string
	CorrelationID string
	CreatedAt     time.Time
}

// RecordInput carries the fields needed to create a new audit log entry.
// Empty optional fields are silently ignored.
type RecordInput struct {
	MerchantID    string
	ActorID       string
	ActorEmail    string
	ActorType     ActorType
	Action        string
	ResourceType  string
	ResourceID    string
	Changes       map[string]any
	IPAddress     string
	CorrelationID string
}

// ListInput controls which audit logs to return.
type ListInput struct {
	MerchantID   string
	ActorID      string
	ResourceType string
	ResourceID   string
	Limit        int
	Cursor       string // created_at:id pair for keyset pagination
}
