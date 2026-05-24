package retention

import (
	"context"
	"strings"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListPolicies(ctx context.Context) ([]Policy, error) {
	return s.repo.ListPolicies(ctx)
}

func (s *Service) UpsertPolicy(ctx context.Context, policy Policy) (Policy, error) {
	policy.UpdatedBy = strings.TrimSpace(policy.UpdatedBy)
	return s.repo.UpsertPolicy(ctx, policy)
}

func (s *Service) ListLegalHolds(ctx context.Context, limit int) ([]LegalHold, error) {
	return s.repo.ListLegalHolds(ctx, limit)
}

func (s *Service) CreateLegalHold(ctx context.Context, in CreateLegalHoldInput) (LegalHold, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	in.CreatedBy = strings.TrimSpace(in.CreatedBy)
	return s.repo.CreateLegalHold(ctx, in)
}

func (s *Service) ReleaseLegalHold(ctx context.Context, holdID string) (LegalHold, error) {
	return s.repo.ReleaseLegalHold(ctx, strings.TrimSpace(holdID))
}

func (s *Service) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	return s.repo.ListRuns(ctx, limit)
}

func (s *Service) RunPolicy(ctx context.Context, in RunInput) (Run, error) {
	in.ActorType = strings.TrimSpace(in.ActorType)
	in.ActorID = strings.TrimSpace(in.ActorID)
	return s.repo.RunPolicy(ctx, in)
}

func (s *Service) RunAll(ctx context.Context, actorType, actorID string) ([]Run, error) {
	return s.repo.RunAll(ctx, strings.TrimSpace(actorType), strings.TrimSpace(actorID))
}
