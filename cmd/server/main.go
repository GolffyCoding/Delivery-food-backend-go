package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/opendelivery/opendelivery/configs"
	"github.com/opendelivery/opendelivery/database"
	"github.com/opendelivery/opendelivery/internal/admin"
	"github.com/opendelivery/opendelivery/internal/auth"
	"github.com/opendelivery/opendelivery/internal/coupon"
	"github.com/opendelivery/opendelivery/internal/driver"
	"github.com/opendelivery/opendelivery/internal/menu"
	"github.com/opendelivery/opendelivery/internal/notification"
	"github.com/opendelivery/opendelivery/internal/order"
	"github.com/opendelivery/opendelivery/internal/outbox"
	"github.com/opendelivery/opendelivery/internal/payment"
	"github.com/opendelivery/opendelivery/internal/restaurant"
	"github.com/opendelivery/opendelivery/internal/review"
	"github.com/opendelivery/opendelivery/internal/support"
	"github.com/opendelivery/opendelivery/internal/tracking"
	"github.com/opendelivery/opendelivery/internal/wallet"
	"github.com/opendelivery/opendelivery/internal/websocket"
	"github.com/opendelivery/opendelivery/pkg/lock"
	"github.com/opendelivery/opendelivery/pkg/middleware"
	"github.com/opendelivery/opendelivery/pkg/realtime"
	"github.com/opendelivery/opendelivery/pkg/response"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := configs.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	ctx := context.Background()

	db, err := database.NewPostgresDB(ctx, cfg.Database, false)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer database.ClosePostgresDB(db)

	if err := database.RunMigrations(ctx, db); err != nil {
		logger.Fatal("failed to run migrations", zap.Error(err))
	}
	logger.Info("database migrations applied")

	rdb, err := database.NewRedisClient(ctx, cfg.Redis)
	if err != nil {
		logger.Fatal("failed to connect to redis", zap.Error(err))
	}
	defer database.CloseRedisClient(rdb)

	validate := validator.New()

	// --- Cross-cutting infra ---
	wsHub := websocket.NewHub(logger)
	authMW := middleware.NewAuthMiddleware(&cfg.JWT, logger)
	redisLocker := lock.NewRedisLocker(rdb)
	outboxRepo := outbox.NewPostgresRepository(db)
	geoCache := driver.NewLocationCache(rdb)

	// Relay events published by other processes (e.g. the worker) to whichever live
	// websocket connection this server instance is holding for the target user.
	realtimeCtx, realtimeCancel := context.WithCancel(context.Background())
	defer realtimeCancel()
	go realtime.Subscribe(realtimeCtx, rdb, wsHub, logger)

	// --- Auth ---
	authRepo := auth.NewPostgresRepository(db, rdb)
	authService := auth.NewService(authRepo, &cfg.JWT)
	authHandler := auth.NewHandler(authService, validate, logger)

	// --- Restaurant ---
	restRepo := restaurant.NewPostgresRepository(db)
	restService := restaurant.NewService(restRepo)
	restHandler := restaurant.NewHandler(restService, validate, logger)
	restChecker := restaurant.NewCheckerAdapter(restRepo)

	// --- Menu ---
	menuRepo := menu.NewPostgresRepository(db)
	menuService := menu.NewService(menuRepo)
	menuHandler := menu.NewHandler(menuService, validate, logger)

	// --- Coupon ---
	couponRepo := coupon.NewPostgresRepository(db)
	couponService := coupon.NewService(couponRepo)
	couponHandler := coupon.NewHandler(couponService, validate, logger)

	// --- Order (depends on restaurant checker, coupon validator, ws hub, redis lock, outbox) ---
	orderRepo := order.NewPostgresRepository(db)
	orderService := order.NewService(orderRepo, restChecker, couponService, wsHub, redisLocker, outboxRepo)
	orderHandler := order.NewHandler(orderService, validate, logger)

	// --- Driver ---
	driverRepo := driver.NewPostgresRepository(db)
	driverService := driver.NewService(driverRepo, geoCache, wsHub)
	driverHandler := driver.NewHandler(driverService, validate, logger)

	// --- Tracking ---
	trackingService := tracking.NewService(driverRepo, wsHub, geoCache, tracking.NewRedisLastLocationStore(rdb))
	trackingHandler := tracking.NewHandler(trackingService, validate, logger)

	// --- Payment ---
	paymentRepo := payment.NewPostgresRepository(db)
	paymentService := payment.NewService(paymentRepo,
		payment.NewCashProvider(),
		payment.NewMockGatewayProvider("mock-card-gateway", payment.MethodCreditCard),
		payment.NewMockGatewayProvider("mock-promptpay-gateway", payment.MethodPromptPay),
		payment.NewMockGatewayProvider("mock-wallet-gateway", payment.MethodWallet),
	)
	paymentHandler := payment.NewHandler(paymentService, validate, logger)

	// --- Wallet ---
	walletRepo := wallet.NewPostgresRepository(db)
	walletService := wallet.NewService(walletRepo)
	walletHandler := wallet.NewHandler(walletService, validate, logger)

	// --- Review (feeds back into restaurant rating) ---
	reviewRepo := review.NewPostgresRepository(db)
	reviewService := review.NewService(reviewRepo, restService)
	reviewHandler := review.NewHandler(reviewService, validate, logger)

	// --- Notification ---
	notifRepo := notification.NewPostgresRepository(db)
	notifService := notification.NewService(notifRepo,
		notification.NewInAppSender(wsHub),
		notification.NewLogSender(notification.TypeEmail),
		notification.NewLogSender(notification.TypeSMS),
	)
	notifHandler := notification.NewHandler(notifService, validate, logger)

	// --- Support (depends on order for context snapshots, ws hub for the live queue) ---
	supportRepo := support.NewPostgresRepository(db)
	supportService := support.NewService(supportRepo, orderService, wsHub)
	supportHandler := support.NewHandler(supportService, validate, logger)

	// --- Websocket ---
	wsHandler := websocket.NewHandler(wsHub, logger)

	// --- Admin ---
	adminHandler := admin.NewHandler(db)

	// --- Fiber app ---
	app := fiber.New(fiber.Config{
		AppName:      "OpenDelivery API",
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
		ErrorHandler: func(c fiber.Ctx, err error) error {
			logger.Error("unhandled error", zap.Error(err), zap.String("path", c.Path()))
			return response.InternalError(c)
		},
	})

	app.Use(middleware.CORS())
	app.Use(middleware.RequestID())
	app.Use(middleware.Logging(logger))
	app.Use(middleware.Recovery(logger))
	app.Use(middleware.RateLimiting(rdb, logger))
	app.Use(middleware.Idempotency(rdb, &cfg.JWT))

	app.Get("/health", func(c fiber.Ctx) error {
		return response.Success(c, fiber.Map{"status": "healthy", "service": "opendelivery"})
	})

	auth.RegisterRoutes(app, authHandler, authMW)
	restaurant.RegisterRoutes(app, restHandler, authMW)
	menu.RegisterRoutes(app, menuHandler, authMW)
	order.RegisterRoutes(app, orderHandler, authMW)
	driver.RegisterRoutes(app, driverHandler, authMW)
	tracking.RegisterRoutes(app, trackingHandler, authMW)
	payment.RegisterRoutes(app, paymentHandler, authMW)
	wallet.RegisterRoutes(app, walletHandler, authMW)
	coupon.RegisterRoutes(app, couponHandler, authMW)
	review.RegisterRoutes(app, reviewHandler, authMW)
	notification.RegisterRoutes(app, notifHandler, authMW)
	support.RegisterRoutes(app, supportHandler, authMW)
	admin.RegisterRoutes(app, adminHandler, authMW)
	websocket.RegisterRoutes(app, wsHandler, authMW)

	// --- Outbox publisher: best-effort, only runs if NATS is reachable ---
	if cfg.NATS.Enabled {
		startOutboxPublisher(ctx, cfg, outboxRepo, logger)
	}

	go func() {
		if err := app.Listen(cfg.Server.Port); err != nil {
			logger.Fatal("failed to start server", zap.Error(err))
		}
	}()
	logger.Info("server listening", zap.String("port", cfg.Server.Port))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logger.Error("server forced to shutdown", zap.Error(err))
	}

	logger.Info("server exited gracefully")
}
