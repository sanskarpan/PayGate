package retention

import (
	"context"
	"log"
	"time"
)

type Worker struct {
	svc      *Service
	interval time.Duration
}

func NewWorker(svc *Service, interval time.Duration) *Worker {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	return &Worker{svc: svc, interval: interval}
}

func (w *Worker) Start(ctx context.Context) {
	if _, err := w.svc.RunAll(ctx, "system", "retention_worker_startup"); err != nil && ctx.Err() == nil {
		log.Printf("retention worker startup run failed: %v", err)
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.svc.RunAll(ctx, "system", "retention_worker"); err != nil && ctx.Err() == nil {
				log.Printf("retention worker run failed: %v", err)
			}
		}
	}
}
