package main

import (
	"context"
	"time"

	"github.com/opendelivery/opendelivery/configs"
	"github.com/opendelivery/opendelivery/internal/outbox"
	"github.com/opendelivery/opendelivery/pkg/eventbus"
	"go.uber.org/zap"
)

// startOutboxPublisher wires the transactional outbox to NATS JetStream. It is
// best-effort: if NATS is unreachable, the API keeps serving requests and order
// rows still land in outbox_messages for the worker/publisher to retry later.
func startOutboxPublisher(ctx context.Context, cfg *configs.Config, repo outbox.Repository, logger *zap.Logger) {
	bus, err := eventbus.NewEventBus(cfg.NATS.URL, logger)
	if err != nil {
		logger.Warn("NATS unavailable, outbox publisher disabled", zap.Error(err))
		return
	}

	worker := outbox.NewPublisherWorker(repo, bus, logger)
	go worker.Run(ctx, 2*time.Second)
	logger.Info("outbox publisher started", zap.String("nats_url", cfg.NATS.URL))
}
