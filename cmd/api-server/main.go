package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/frstrtr/mongotron/internal/api/handlers"
	"github.com/frstrtr/mongotron/internal/api/websocket"
	"github.com/frstrtr/mongotron/internal/blockchain/client"
	"github.com/frstrtr/mongotron/internal/config"
	"github.com/frstrtr/mongotron/internal/storage"
	"github.com/frstrtr/mongotron/internal/subscription"
	"github.com/frstrtr/mongotron/pkg/logger"
	wsfiber "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	fiberlogger "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

const version = "1.0.0"

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(cfg.Logging)
	log.Info().Str("version", version).Msg("Starting MongoTron API Server")

	// Initialize database
	dbCfg := storage.Config{
		URI:            cfg.Database.MongoDB.URI,
		Database:       cfg.Database.MongoDB.Database,
		MaxPoolSize:    100,
		MinPoolSize:    10,
		MaxIdleTime:    time.Minute * 10,
		ConnectTimeout: time.Second * 10,
	}
	db, err := storage.NewDatabase(dbCfg, &log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		db.Close(ctx)
	}()

	// Initialize Tron client
	tronCfg := client.Config{
		Host:            cfg.Blockchain.Tron.Node.Host,
		Port:            cfg.Blockchain.Tron.Node.Port,
		Timeout:         cfg.Blockchain.Tron.Connection.Timeout,
		MaxRetries:      cfg.Blockchain.Tron.Connection.MaxRetries,
		BackoffInterval: cfg.Blockchain.Tron.Connection.BackoffInterval,
		KeepAlive:       cfg.Blockchain.Tron.Connection.KeepAlive,
	}
	tronClient, err := client.NewTronClient(tronCfg, &log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize Tron client")
	}
	defer tronClient.Close()

	// Initialize subscription manager
	manager := subscription.NewManager(db, tronClient, &log)
	if err := manager.Start(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start subscription manager")
	}
	defer manager.Stop()

	// Initialize WebSocket hub
	hub := websocket.NewHub(manager.GetEventRouter(), &log)
	go hub.Run()
	defer hub.Stop()

	// Initialize Fiber app
	app := fiber.New(fiber.Config{
		AppName:               "MongoTron API v" + version,
		DisableStartupMessage: false,
		EnablePrintRoutes:     true,
		ReadTimeout:           30 * time.Second,
		WriteTimeout:          30 * time.Second,
		IdleTimeout:           120 * time.Second,
		ErrorHandler:          customErrorHandler,
	})

	// Middleware
	app.Use(recover.New())
	app.Use(fiberlogger.New(fiberlogger.Config{
		Format: "[${time}] ${status} - ${latency} ${method} ${path}\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))
	app.Use(limiter.New(limiter.Config{
		Max:        100,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   "rate_limit_exceeded",
				"message": "Too many requests, please try again later",
			})
		},
	}))

	// Initialize handlers
	subscriptionHandler := handlers.NewSubscriptionHandler(manager)
	eventHandler := handlers.NewEventHandler(db.EventRepo)
	healthHandler := handlers.NewHealthHandler(manager, version)
	wsHandler := handlers.NewWebSocketHandler(hub, manager)

	// Routes
	api := app.Group("/api/v1")

	// Health endpoints
	api.Get("/health", healthHandler.HealthCheck)
	api.Get("/ready", healthHandler.ReadinessCheck)
	api.Get("/live", healthHandler.LivenessCheck)

	// Subscription endpoints
	api.Post("/subscriptions", subscriptionHandler.CreateSubscription)
	api.Get("/subscriptions", subscriptionHandler.ListSubscriptions)
	api.Get("/subscriptions/:id", subscriptionHandler.GetSubscription)
	api.Delete("/subscriptions/:id", subscriptionHandler.DeleteSubscription)

	// Event endpoints
	api.Get("/events", eventHandler.ListEvents)
	api.Get("/events/:id", eventHandler.GetEvent)
	api.Get("/events/tx/:hash", eventHandler.GetEventByTransactionHash)

	// WebSocket endpoint
	api.Use("/events/stream", handlers.WebSocketMiddleware())
	api.Get("/events/stream/:subscriptionId", wsfiber.New(wsHandler.StreamEvents))

	// Root endpoint
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service": "MongoTron API",
			"version": version,
			"status":  "running",
			"endpoints": fiber.Map{
				"health":        "/api/v1/health",
				"subscriptions": "/api/v1/subscriptions",
				"events":        "/api/v1/events",
				"websocket":     "/api/v1/events/stream/:subscriptionId",
			},
		})
	})

	// Get listen address
	listenAddr := fmt.Sprintf(":%d", cfg.Server.Port)
	if cfg.Server.Host != "" {
		listenAddr = fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	}

	// Start server in goroutine
	go func() {
		log.Info().
			Str("address", listenAddr).
			Msg("Starting HTTP server")

		if err := app.Listen(listenAddr); err != nil {
			log.Fatal().Err(err).Msg("Server failed to start")
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	// Graceful shutdown
	log.Info().Msg("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Error().Err(err).Msg("Server shutdown error")
	}

	log.Info().Msg("Server stopped gracefully")
}

// customErrorHandler handles errors in a consistent format
func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	return c.Status(code).JSON(fiber.Map{
		"error":   fiber.ErrInternalServerError.Message,
		"message": err.Error(),
		"status":  code,
	})
}
