package dispute

import "context"

// Repository defines storage operations for the dispute service.
type Repository interface {
	// Create inserts a new dispute record and emits a dispute.created outbox event.
	Create(ctx context.Context, d Dispute) (Dispute, error)

	// GetByID returns a dispute by ID scoped to the merchant.
	GetByID(ctx context.Context, merchantID, id string) (Dispute, error)

	// List returns disputes for a merchant ordered by created_at DESC, limited to limit rows.
	List(ctx context.Context, merchantID string, limit int) ([]Dispute, error)

	// UpdateStatus transitions the dispute to status, sets resolved_at for terminal states,
	// and emits a dispute.updated outbox event.
	UpdateStatus(ctx context.Context, merchantID, id string, status DisputeState, notes string) (Dispute, error)

	// SubmitEvidence stores the evidence payload and stamps evidence_submitted_at.
	SubmitEvidence(ctx context.Context, merchantID, id string, evidence map[string]any) (Dispute, error)
}
