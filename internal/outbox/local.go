package outbox

import (
	"context"
	"log/slog"
)

// LocalHandler handles a published envelope in-process.
type LocalHandler func(topic, key string, payload []byte) error

// LocalPublisher routes outbox events directly to an in-process handler.
type LocalPublisher struct {
	handler LocalHandler
}

func NewLocalPublisher(handler LocalHandler) *LocalPublisher {
	return &LocalPublisher{handler: handler}
}

func (p *LocalPublisher) Publish(_ context.Context, topic, key string, payload []byte) error {
	if p.handler == nil {
		return nil
	}
	return p.handler(topic, key, payload)
}

func (p *LocalPublisher) Close() error { return nil }

// FallbackPublisher tries the primary publisher first, then falls back to a
// secondary publisher when the primary publish fails.
type FallbackPublisher struct {
	primary  Publisher
	fallback *LocalPublisher
	logger   *slog.Logger
}

func NewFallbackPublisher(primary Publisher, fallback *LocalPublisher, logger *slog.Logger) *FallbackPublisher {
	if logger == nil {
		logger = slog.Default()
	}
	return &FallbackPublisher{primary: primary, fallback: fallback, logger: logger}
}

func (p *FallbackPublisher) Publish(ctx context.Context, topic string, key string, payload []byte) error {
	if p.primary != nil {
		if err := p.primary.Publish(ctx, topic, key, payload); err == nil {
			return nil
		} else if p.fallback == nil {
			return err
		} else {
			p.logger.Warn("primary outbox publish failed, falling back to local dispatcher", "topic", topic, "error", err)
		}
	}
	if p.fallback == nil {
		return nil
	}
	return p.fallback.Publish(ctx, topic, key, payload)
}

func (p *FallbackPublisher) Close() error {
	if p.primary != nil {
		return p.primary.Close()
	}
	return nil
}
