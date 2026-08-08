// Package realtime bridges server processes over Redis Pub/Sub so an event raised in
// one process (e.g. the background worker) reaches a user's live websocket connection
// even though that connection is held by a different process (the API server). Redis
// is already a required dependency everywhere this project runs, so this needs no new
// infrastructure, unlike the optional/feature-flagged NATS event bus.
package realtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const channel = "realtime:push"

// Envelope is published on the shared channel; every process subscribed decides
// whether an event is meant for a user it currently holds a connection for.
type Envelope struct {
	UserID  uuid.UUID   `json:"user_id"`
	Message interface{} `json:"message"`
}

type Publisher struct {
	rdb *redis.Client
}

func NewPublisher(rdb *redis.Client) *Publisher {
	return &Publisher{rdb: rdb}
}

// PublishToUser fans an arbitrary payload out to every process; whichever one holds
// a live connection for userID is responsible for actually delivering it.
func (p *Publisher) PublishToUser(ctx context.Context, userID uuid.UUID, message interface{}) error {
	data, err := json.Marshal(Envelope{UserID: userID, Message: message})
	if err != nil {
		return fmt.Errorf("marshal realtime envelope: %w", err)
	}
	return p.rdb.Publish(ctx, channel, data).Err()
}

// Deliverer is implemented by the websocket hub: it knows whether it holds a live
// connection for a user and how to push a message down it.
type Deliverer interface {
	SendToUserCtx(ctx context.Context, userID uuid.UUID, payload interface{}) error
}

// Subscribe blocks, relaying every published envelope to deliverer until ctx is
// cancelled. Call it in its own goroutine once per process that hosts a websocket hub.
func Subscribe(ctx context.Context, rdb *redis.Client, deliverer Deliverer, logger *zap.Logger) {
	sub := rdb.Subscribe(ctx, channel)
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var env Envelope
			if err := json.Unmarshal([]byte(msg.Payload), &env); err != nil {
				logger.Warn("realtime: dropping malformed envelope", zap.Error(err))
				continue
			}
			if err := deliverer.SendToUserCtx(ctx, env.UserID, env.Message); err != nil {
				logger.Warn("realtime: delivery failed", zap.Error(err), zap.String("user_id", env.UserID.String()))
			}
		}
	}
}
