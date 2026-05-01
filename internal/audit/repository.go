package audit

import "context"

// Repository defines the storage contract for audit logs.
// Implementations must be append-only; no UPDATE or DELETE.
type Repository interface {
	// Create persists a new audit log entry and returns it with CreatedAt set.
	Create(ctx context.Context, log Log) (Log, error)
	// List returns audit logs for a merchant matching the given filter.
	List(ctx context.Context, in ListInput) ([]Log, error)
}
