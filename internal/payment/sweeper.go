package payment

import (
	"context"
	"log/slog"
	"time"
)

type Sweeper struct {
	svc      *Service
	interval time.Duration
	logger   *slog.Logger
}

func NewSweeper(svc *Service, interval time.Duration, logger *slog.Logger) *Sweeper {
	if logger == nil {
		logger = slog.Default()
	}
	return &Sweeper{svc: svc, interval: interval, logger: logger}
}

func (s *Sweeper) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			captured, err := s.svc.repo.AutoCaptureDue(ctx)
			if err != nil {
				s.logger.Error("auto-capture sweep failed", "error", err)
			} else if captured > 0 {
				s.logger.Info("auto-captured payments", "count", captured)
			}

			expired, err := s.svc.repo.ExpireAuthorizationWindow(ctx, 5*24*time.Hour)
			if err != nil {
				s.logger.Error("auth-expiry sweep failed", "error", err)
			} else if expired > 0 {
				s.logger.Info("auto-refunded authorizations", "count", expired)
			}
		}
	}
}
