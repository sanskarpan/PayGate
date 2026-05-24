package retention

import "context"

type Repository interface {
	ListPolicies(ctx context.Context) ([]Policy, error)
	UpsertPolicy(ctx context.Context, policy Policy) (Policy, error)
	ListLegalHolds(ctx context.Context, limit int) ([]LegalHold, error)
	CreateLegalHold(ctx context.Context, in CreateLegalHoldInput) (LegalHold, error)
	ReleaseLegalHold(ctx context.Context, holdID string) (LegalHold, error)
	ListRuns(ctx context.Context, limit int) ([]Run, error)
	RunPolicy(ctx context.Context, in RunInput) (Run, error)
	RunAll(ctx context.Context, actorType, actorID string) ([]Run, error)
}
