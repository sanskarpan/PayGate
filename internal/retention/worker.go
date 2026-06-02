package retention

import (
	"context"
	"log/slog"
	"time"
)

type Worker struct {
	svc      *Service
	interval time.Duration
	logger   *slog.Logger
}

func NewWorker(svc *Service, interval time.Duration, logger *slog.Logger) *Worker {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{svc: svc, interval: interval, logger: logger}
}

func (w *Worker) Start(ctx context.Context) {
	if _, err := w.svc.RunAll(ctx, "system", "retention_worker_startup"); err != nil && ctx.Err() == nil {
		w.logger.Error("retention worker startup run failed", "error", err)
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.svc.RunAll(ctx, "system", "retention_worker"); err != nil && ctx.Err() == nil {
				w.logger.Error("retention worker run failed", "error", err)
			}
		}
	}
}
