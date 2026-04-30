package order

import (
	"context"
	"log/slog"
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
			if count > 0 {
				s.logger.Info("expired orders", "count", count)
			}
		}
	}
}
