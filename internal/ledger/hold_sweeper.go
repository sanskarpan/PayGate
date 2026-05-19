package ledger

import (
	"context"
	"log/slog"
	"time"
)

type HoldSweeper struct {
	svc      *Service
	interval time.Duration
	logger   *slog.Logger
}

func NewHoldSweeper(svc *Service, interval time.Duration, logger *slog.Logger) *HoldSweeper {
	if interval <= 0 {
		interval = time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HoldSweeper{svc: svc, interval: interval, logger: logger}
}

func (s *HoldSweeper) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			expired, err := s.svc.ExpireDueHolds(ctx, 100)
			if err != nil {
				s.logger.Error("expire ledger holds failed", "error", err)
				continue
			}
			if expired > 0 {
				s.logger.Info("expired ledger holds", "count", expired)
			}
		}
	}
}
