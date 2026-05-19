package saga

import (
	"context"
	"log/slog"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/idgen"
)

type Worker struct {
	svc      *Service
	interval time.Duration
	logger   *slog.Logger
}

func NewWorker(svc *Service, interval time.Duration, logger *slog.Logger) *Worker {
	if interval <= 0 {
		interval = time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{svc: svc, interval: interval, logger: logger}
}

func (w *Worker) Start(ctx context.Context) {
	leaseOwner := idgen.New("swrk")
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.drain(ctx, leaseOwner); err != nil && ctx.Err() == nil {
				w.logger.Error("saga worker drain failed", "error", err)
			}
		}
	}
}

func (w *Worker) drain(ctx context.Context, leaseOwner string) error {
	for {
		processed, err := w.svc.ProcessNextTimeout(ctx, leaseOwner)
		if err != nil {
			return err
		}
		if processed {
			continue
		}
		processed, err = w.svc.RunNext(ctx, leaseOwner)
		if err != nil {
			return err
		}
		if !processed {
			return nil
		}
	}
}
