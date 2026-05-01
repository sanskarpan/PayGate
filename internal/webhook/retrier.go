package webhook

import (
	"context"
	"log/slog"
	"time"
)

// RetryWorker polls for failed delivery attempts due for retry and re-delivers them.
// It runs as a background goroutine and uses DB-level locking (FOR UPDATE SKIP LOCKED)
// to safely co-exist with multiple instances.
type RetryWorker struct {
	svc      *Service
	interval time.Duration
	batchSz  int
	logger   *slog.Logger
}

// NewRetryWorker creates a RetryWorker that polls every interval.
func NewRetryWorker(svc *Service, interval time.Duration, logger *slog.Logger) *RetryWorker {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &RetryWorker{svc: svc, interval: interval, batchSz: 50, logger: logger}
}

// Start runs the retry loop until ctx is cancelled.
func (w *RetryWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := w.svc.RetryPendingDeliveries(ctx, w.batchSz)
			if err != nil {
				w.logger.Error("webhook retry batch failed", "error", err)
				continue
			}
			if n > 0 {
				w.logger.Info("webhook retry batch processed", "count", n)
			}
		}
	}
}
