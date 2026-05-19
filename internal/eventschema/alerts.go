package eventschema

import (
	"context"
	"log/slog"
	"time"

	"github.com/sanskarpan/PayGate/internal/common/metrics"
)

type AlertChecker struct {
	svc      *Service
	interval time.Duration
	logger   *slog.Logger
}

func NewAlertChecker(svc *Service, interval time.Duration, logger *slog.Logger) *AlertChecker {
	if interval <= 0 {
		interval = time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AlertChecker{svc: svc, interval: interval, logger: logger}
}

func (c *AlertChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.runOnce(ctx); err != nil && ctx.Err() == nil {
				c.logger.Error("event schema alert check failed", "error", err)
			}
		}
	}
}

func (c *AlertChecker) runOnce(ctx context.Context) error {
	alerts, err := c.svc.ListDeprecatedVersionAlerts(ctx)
	if err != nil {
		return err
	}
	metrics.ResetDeprecatedSchemaAlerts()
	for _, alert := range alerts {
		metrics.RecordDeprecatedSchemaAlert(alert.Subject, alert.FromVersion, alert.ToVersion, alert.ConsumerName)
	}
	return nil
}
