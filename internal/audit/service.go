package audit

import (
	"context"
	"log/slog"
)

// Service records audit events and exposes the query API.
// Record() is fire-and-forget: errors are logged but never returned to the
// caller so that audit failures never block the primary operation.
type Service struct {
	repo   Repository
	logger *slog.Logger
}

func NewService(repo Repository, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, logger: logger}
}

// Record creates an audit log entry asynchronously.
// It must be called after the primary state change succeeds.
func (s *Service) Record(ctx context.Context, in RecordInput) {
	go func() {
		l := Log{
			MerchantID:    in.MerchantID,
			ActorID:       in.ActorID,
			ActorEmail:    in.ActorEmail,
			ActorType:     in.ActorType,
			Action:        in.Action,
			ResourceType:  in.ResourceType,
			ResourceID:    in.ResourceID,
			Changes:       in.Changes,
			IPAddress:     in.IPAddress,
			CorrelationID: in.CorrelationID,
		}
		if _, err := s.repo.Create(ctx, l); err != nil {
			s.logger.Error("audit record failed",
				"action", in.Action,
				"resource_type", in.ResourceType,
				"resource_id", in.ResourceID,
				"error", err,
			)
		}
	}()
}

// List returns audit logs for the given merchant matching the filter.
func (s *Service) List(ctx context.Context, in ListInput) ([]Log, error) {
	return s.repo.List(ctx, in)
}
