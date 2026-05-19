package eventschema

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/sanskarpan/PayGate/internal/outbox"
)

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

func (s *Service) CreateSchema(ctx context.Context, in CreateSchemaInput) (Schema, error) {
	return s.repo.CreateSchema(ctx, in)
}

func (s *Service) ListSchemas(ctx context.Context) ([]Schema, error) {
	return s.repo.ListSchemas(ctx)
}

func (s *Service) GetSchema(ctx context.Context, subject string) (Schema, []Version, []CompatibilityCheck, error) {
	item, err := s.repo.GetSchema(ctx, subject)
	if err != nil {
		return Schema{}, nil, nil, err
	}
	versions, err := s.repo.ListVersions(ctx, subject)
	if err != nil {
		return Schema{}, nil, nil, err
	}
	checks, err := s.repo.ListChecks(ctx, subject, 50)
	if err != nil {
		return Schema{}, nil, nil, err
	}
	return item, versions, checks, nil
}

func (s *Service) RegisterVersion(ctx context.Context, in CreateVersionInput) (Version, []CompatibilityCheck, error) {
	if err := ValidateDocument(in.Schema); err != nil {
		return Version{}, nil, err
	}

	var checks []CompatibilityCheck
	previous, err := s.repo.GetActiveVersion(ctx, in.Subject)
	switch {
	case err == nil:
		rawChecks := CheckCompatibility(previous.Schema, in.Schema)
		checks = make([]CompatibilityCheck, 0, len(rawChecks))
		for _, check := range rawChecks {
			check.Subject = in.Subject
			check.CandidateVersion = in.Version
			check.BaselineVersion = previous.Version
			checks = append(checks, check)
			if !check.Compatible {
				return Version{}, checks, ErrIncompatibleSchema
			}
		}
	case errors.Is(err, ErrNoActiveSchemaVersion):
		// Initial version is allowed.
	default:
		return Version{}, nil, err
	}

	version, err := s.repo.CreateVersion(ctx, in, checks)
	return version, checks, err
}

func (s *Service) GetVersion(ctx context.Context, subject, version string) (Version, error) {
	return s.repo.GetVersion(ctx, subject, version)
}

func (s *Service) ActivateVersion(ctx context.Context, in ActivateVersionInput) (Version, error) {
	version, err := s.repo.GetVersion(ctx, in.Subject, in.Version)
	if err != nil {
		return Version{}, err
	}
	if err := ValidateDocument(version.Schema); err != nil {
		return Version{}, err
	}
	return s.repo.ActivateVersion(ctx, in)
}

func (s *Service) CompareVersions(ctx context.Context, in CompareVersionsInput) ([]CompatibilityCheck, error) {
	fromVersion, err := s.repo.GetVersion(ctx, in.Subject, in.FromVersion)
	if err != nil {
		return nil, err
	}
	toVersion, err := s.repo.GetVersion(ctx, in.Subject, in.ToVersion)
	if err != nil {
		return nil, err
	}
	checks := CheckCompatibility(fromVersion.Schema, toVersion.Schema)
	for i := range checks {
		checks[i].Subject = in.Subject
		checks[i].CandidateVersion = in.ToVersion
		checks[i].BaselineVersion = in.FromVersion
	}
	return checks, nil
}

func (s *Service) CreateRollout(ctx context.Context, in CreateRolloutInput) (Rollout, error) {
	if in.FromVersion == in.ToVersion {
		return Rollout{}, fmt.Errorf("%w: rollout source and target versions must differ", ErrInvalidSchemaRollout)
	}
	active, err := s.repo.GetActiveVersion(ctx, in.Subject)
	if err != nil {
		return Rollout{}, err
	}
	if active.Version != in.FromVersion {
		return Rollout{}, fmt.Errorf("%w: from_version must match the active schema version", ErrInvalidSchemaRollout)
	}
	target, err := s.repo.GetVersion(ctx, in.Subject, in.ToVersion)
	if err != nil {
		return Rollout{}, err
	}
	if target.Status == StatusRetired {
		return Rollout{}, fmt.Errorf("%w: target version is retired", ErrInvalidSchemaRollout)
	}
	return s.repo.CreateRollout(ctx, in)
}

func (s *Service) AckRollout(ctx context.Context, in AckRolloutInput) (RolloutConsumer, error) {
	rollout, err := s.repo.GetRollout(ctx, in.RolloutID)
	if err != nil {
		return RolloutConsumer{}, err
	}
	if in.AcknowledgedVersion != rollout.FromVersion && in.AcknowledgedVersion != rollout.ToVersion {
		return RolloutConsumer{}, fmt.Errorf("%w: acknowledged_version must match a rollout version", ErrInvalidSchemaRollout)
	}
	return s.repo.AckRollout(ctx, in)
}

func (s *Service) GetRollout(ctx context.Context, rolloutID string) (Rollout, error) {
	return s.repo.GetRollout(ctx, rolloutID)
}

func (s *Service) ResolvePublishVersions(ctx context.Context, subject string) ([]outbox.PublishSchemaVersion, error) {
	active, err := s.repo.GetActiveVersion(ctx, subject)
	if err != nil {
		return nil, err
	}
	versions := []outbox.PublishSchemaVersion{{Subject: subject, Version: active.Version}}

	rollout, err := s.repo.GetActiveRollout(ctx, subject)
	if err == nil {
		if rollout.FromVersion != active.Version {
			s.logger.Warn("schema rollout active against non-active source version", "subject", subject, "active_version", active.Version, "from_version", rollout.FromVersion)
			return versions, nil
		}
		return []outbox.PublishSchemaVersion{
			{Subject: subject, Version: rollout.FromVersion},
			{Subject: subject, Version: rollout.ToVersion},
		}, nil
	}
	if errors.Is(err, ErrSchemaRolloutNotFound) {
		return versions, nil
	}
	return nil, err
}

func (s *Service) ListDeprecatedVersionAlerts(ctx context.Context) ([]DeprecatedVersionAlert, error) {
	return s.repo.ListDeprecatedVersionAlerts(ctx)
}
