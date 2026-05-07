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
// A detached context is used so that HTTP request cancellation does not
// prevent the audit entry from being persisted.
func (s *Service) Record(ctx context.Context, in RecordInput) {
	// Detach from the request context: the HTTP handler may return (and thus
	// cancel ctx) before this goroutine runs its INSERT.
	detached := context.WithoutCancel(ctx)
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
		if _, err := s.repo.Create(detached, l); err != nil {
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
