package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/opendelivery/opendelivery/configs"
	"github.com/opendelivery/opendelivery/database"
	"github.com/opendelivery/opendelivery/internal/driver"
	"github.com/opendelivery/opendelivery/internal/notification"
	"github.com/opendelivery/opendelivery/internal/order"
	"github.com/opendelivery/opendelivery/internal/outbox"
	"github.com/opendelivery/opendelivery/internal/restaurant"
	"github.com/opendelivery/opendelivery/pkg/eventbus"
	"github.com/opendelivery/opendelivery/pkg/realtime"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := configs.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.NewPostgresDB(ctx, cfg.Database, false)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer database.ClosePostgresDB(db)

	rdb, err := database.NewRedisClient(ctx, cfg.Redis)
	if err != nil {
		logger.Fatal("failed to connect to redis", zap.Error(err))
	}
	defer database.CloseRedisClient(rdb)

	orderRepo := order.NewPostgresRepository(db)
	outboxRepo := outbox.NewPostgresRepository(db)
	restRepo := restaurant.NewPostgresRepository(db)
	driverRepo := driver.NewPostgresRepository(db)
	geoCache := driver.NewLocationCache(rdb)

	// The worker process holds no websocket connections of its own, so in_app
	// notifications are relayed through Redis Pub/Sub: whichever API server instance
	// currently holds the target user's live connection delivers it in real time
	// (see pkg/realtime and cmd/server/main.go's Subscribe side).
	realtimePub := realtime.NewPublisher(rdb)
	notifRepo := notification.NewPostgresRepository(db)
	notifService := notification.NewService(notifRepo,
		notification.NewInAppSender(realtimePushAdapter{pub: realtimePub}),
		notification.NewLogSender(notification.TypeEmail),
		notification.NewLogSender(notification.TypeSMS),
	)
	driverService := driver.NewService(driverRepo, geoCache, driverNotifyAdapter{repo: driverRepo, notif: notifService})

	go runOrderTimeoutWorker(ctx, orderRepo, logger)
	go runReadyOrderTimeoutWorker(ctx, orderRepo, restRepo, driverService, notifService, logger)

	if cfg.NATS.Enabled {
		if bus, err := eventbus.NewEventBus(cfg.NATS.URL, logger); err == nil {
			publisher := outbox.NewPublisherWorker(outboxRepo, bus, logger)
			go publisher.Run(ctx, 2*time.Second)
			logger.Info("outbox publisher worker started")
		} else {
			logger.Warn("NATS unavailable, outbox publisher disabled in worker", zap.Error(err))
		}
	}

	logger.Info("workers started successfully")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutting down workers...")
	cancel()
}

// runOrderTimeoutWorker auto-cancels orders that have sat in "pending" too long
// without a restaurant accepting them, so customers aren't left hanging forever.
func runOrderTimeoutWorker(ctx context.Context, repo order.Repository, logger *zap.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	const timeout = 10 * time.Minute

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stale, err := repo.GetStalePending(ctx, time.Now().Add(-timeout))
			if err != nil {
				logger.Error("order timeout sweep failed", zap.Error(err))
				continue
			}
			for _, stale := range stale {
				fresh, err := repo.GetByID(ctx, stale.ID)
				if err != nil {
					continue
				}
				if err := order.ValidateTransition(fresh.Status, order.StatusCancelled); err != nil {
					continue
				}

				now := time.Now()
				fresh.Status = order.StatusCancelled
				fresh.CancelledAt = &now
				fresh.CancelReason = "auto-cancelled: no restaurant response within timeout"

				if err := repo.UpdateWithOptimisticLock(ctx, fresh); err != nil {
					logger.Warn("failed to auto-cancel stale order", zap.String("order_id", fresh.ID.String()), zap.Error(err))
					continue
				}
				logger.Info("auto-cancelled stale order", zap.String("order_id", fresh.ID.String()))
			}
		}
	}
}

// runReadyOrderTimeoutWorker handles the "food is ready but no driver ever showed up"
// scenario reviews repeatedly flagged: the customer was left with zero visibility and
// the restaurant had no way to escalate. Every sweep it re-broadcasts the delivery to
// the nearest available driver and, once the delay drags on, proactively tells the
// customer and the restaurant what is happening instead of leaving them guessing.
func runReadyOrderTimeoutWorker(
	ctx context.Context,
	orderRepo order.Repository,
	restRepo restaurant.Repository,
	driverService *driver.Service,
	notifService *notification.Service,
	logger *zap.Logger,
) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	const rebroadcastAfter = 5 * time.Minute
	const escalateAfter = 10 * time.Minute

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stale, err := orderRepo.GetStaleReady(ctx, time.Now().Add(-rebroadcastAfter))
			if err != nil {
				logger.Error("ready-order timeout sweep failed", zap.Error(err))
				continue
			}
			for _, o := range stale {
				rest, err := restRepo.GetByID(ctx, o.RestaurantID)
				if err != nil {
					logger.Warn("ready-order sweep: restaurant lookup failed", zap.String("order_id", o.ID.String()), zap.Error(err))
					continue
				}

				match, assignErr := driverService.AutoAssign(ctx, o.ID, rest.Latitude, rest.Longitude)
				if assignErr == nil {
					logger.Info("re-broadcast ready order to nearest driver",
						zap.String("order_id", o.ID.String()), zap.String("driver_id", match.DriverID.String()))
					continue
				}

				logger.Warn("no driver available for ready order", zap.String("order_id", o.ID.String()), zap.Error(assignErr))

				if o.PreparedAt == nil || time.Since(*o.PreparedAt) < escalateAfter {
					continue
				}

				_, _ = notifService.Send(ctx, notification.SendRequest{
					UserID:   o.CustomerID,
					Type:     notification.TypeInApp,
					Title:    "Your delivery is delayed",
					Body:     "We're still looking for a driver for order " + o.OrderNumber + ". We're sorry for the wait and are on it.",
					Priority: notification.PriorityHigh,
					Data:     map[string]interface{}{"order_id": o.ID.String()},
				})
				_, _ = notifService.Send(ctx, notification.SendRequest{
					UserID:   rest.MerchantID,
					Type:     notification.TypeInApp,
					Title:    "No driver available",
					Body:     "Order " + o.OrderNumber + " has been ready for over 10 minutes with no driver assigned. Consider contacting support.",
					Priority: notification.PriorityHigh,
					Data:     map[string]interface{}{"order_id": o.ID.String()},
				})
			}
		}
	}
}

// realtimePushAdapter satisfies notification.PushNotifier by fanning a push out over
// Redis instead of a local in-process websocket hub, since the worker holds no
// connections itself.
type realtimePushAdapter struct {
	pub *realtime.Publisher
}

func (a realtimePushAdapter) SendToUserCtx(ctx context.Context, userID uuid.UUID, payload interface{}) error {
	return a.pub.PublishToUser(ctx, userID, payload)
}

// driverNotifyAdapter bridges driver.EventNotifier to the notification service so the
// worker process (which has no live websocket hub) can still raise a real-time
// notification for the driver being matched to a delivery.
type driverNotifyAdapter struct {
	repo  driver.Repository
	notif *notification.Service
}

func (a driverNotifyAdapter) NotifyDriver(ctx context.Context, driverID uuid.UUID, eventType string, data interface{}) error {
	d, err := a.repo.GetByID(ctx, driverID)
	if err != nil {
		return err
	}
	_, err = a.notif.Send(ctx, notification.SendRequest{
		UserID:   d.UserID,
		Type:     notification.TypeInApp,
		Title:    "New delivery request",
		Body:     "You've been matched to a nearby order. Open the app to accept it.",
		Priority: notification.PriorityHigh,
		Data:     map[string]interface{}{"event": eventType},
	})
	return err
}
