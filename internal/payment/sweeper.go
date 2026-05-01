package payment

import (
	"context"
	"log/slog"
	"math/rand/v2"
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
	// Random jitter up to one full interval so multiple sweeper instances
	// started simultaneously do not all tick at the same wall-clock time.
	jitter := time.Duration(rand.Int64N(int64(s.interval)))
	select {
	case <-ctx.Done():
		return
	case <-time.After(jitter):
	}

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
			} else {
				s.logger.Info("auto-capture sweep done", "count", captured)
			}

			expired, err := s.svc.repo.ExpireAuthorizationWindow(ctx, 5*24*time.Hour)
			if err != nil {
				s.logger.Error("auth-expiry sweep failed", "error", err)
			} else {
				s.logger.Info("auth-expiry sweep done", "count", expired)
			}
		}
	}
}
