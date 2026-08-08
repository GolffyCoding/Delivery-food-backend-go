package eventbus

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// EventBus wraps a NATS JetStream connection for at-least-once event delivery.
type EventBus struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
	logger *zap.Logger
}

func NewEventBus(natsURL string, logger *zap.Logger) (*EventBus, error) {
	nc, err := nats.Connect(natsURL, nats.ReconnectWait(2*time.Second), nats.MaxReconnects(5))
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("init JetStream: %w", err)
	}

	// Ensure a stream exists to capture outbox-published subjects.
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "OPENDELIVERY_EVENTS",
		Subjects: []string{"order.>", "driver.>", "notification.>"},
	})
	if err != nil && err != nats.ErrStreamNameAlreadyInUse {
		logger.Warn("failed to ensure JetStream stream", zap.Error(err))
	}

	return &EventBus{nc: nc, js: js, logger: logger}, nil
}

func (eb *EventBus) Publish(subject string, data []byte, dedupeID string) error {
	_, err := eb.js.Publish(subject, data, nats.MsgId(dedupeID))
	if err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}
	return nil
}

func (eb *EventBus) Subscribe(subject, queueGroup string, handler func(subject string, data []byte)) error {
	_, err := eb.js.QueueSubscribe(subject, queueGroup, func(msg *nats.Msg) {
		handler(msg.Subject, msg.Data)
		_ = msg.Ack()
	}, nats.Durable(queueGroup), nats.ManualAck())
	return err
}

func (eb *EventBus) Close() {
	if eb.nc != nil {
		eb.nc.Close()
	}
}
