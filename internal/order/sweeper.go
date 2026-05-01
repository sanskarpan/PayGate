package order

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"
)

type ExpirySweeper struct {
	svc      *Service
	interval time.Duration
	logger   *slog.Logger
}

func NewExpirySweeper(svc *Service, interval time.Duration, logger *slog.Logger) *ExpirySweeper {
	if logger == nil {
		logger = slog.Default()
	}
	return &ExpirySweeper{svc: svc, interval: interval, logger: logger}
}

func (s *ExpirySweeper) Start(ctx context.Context) {
	// Random jitter up to one full interval so concurrent sweeper instances
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
			count, err := s.svc.repo.ExpireDueOrders(ctx)
			if err != nil {
				s.logger.Error("order expiry sweep failed", "error", err)
				continue
			}
			s.logger.Info("order expiry sweep done", "count", count)
		}
	}
}
