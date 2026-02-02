// Package main is the entry point for the finance service.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	grpcdelivery "github.com/mutugading/goapps-backend/services/finance/internal/delivery/grpc"
	httpdelivery "github.com/mutugading/goapps-backend/services/finance/internal/delivery/httpdelivery"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	redisinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/redis"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/tracing"
)

func main() {
	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("Service failed")
	}
}

// run contains the main application logic, separated for cleaner error handling.
func run() error {
	setupLogger()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log.Info().
		Str("service", cfg.App.Name).
		Str("version", cfg.App.Version).
		Str("environment", cfg.App.Env).
		Msg("Starting finance service")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup tracing (optional)
	cleanupTracing := setupTracing(ctx, cfg)
	defer cleanupTracing()

	// Setup database
	db, err := setupDatabase(cfg)
	if err != nil {
		return err
	}
	defer closeDatabase(db)

	// Setup Redis (optional - graceful degradation)
	redisClient, uomCache := setupRedis(cfg)
	if redisClient != nil {
		defer closeRedis(redisClient)
	}

	// Setup repository
	uomRepo := postgres.NewUOMRepository(db)

	// Setup gRPC handler
	uomHandler, err := grpcdelivery.NewUOMHandler(uomRepo, uomCache)
	if err != nil {
		return err
	}

	// Setup and start servers
	return startServers(ctx, cfg, uomHandler)
}

// setupLogger configures the application logger.
func setupLogger() {
	zerolog.TimeFieldFormat = time.RFC3339
	if os.Getenv("APP_ENV") == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}

// setupTracing initializes tracing and returns a cleanup function.
func setupTracing(ctx context.Context, cfg *config.Config) func() {
	tracingProvider, err := tracing.NewProvider(ctx, &cfg.Tracing, cfg.App.Name, cfg.App.Version)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to setup tracing, continuing without it")
		return func() {}
	}

	if tracingProvider == nil {
		return func() {}
	}

	return func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := tracingProvider.Shutdown(shutdownCtx); err != nil {
			log.Warn().Err(err).Msg("Failed to shutdown tracing provider")
		}
	}
}

// setupDatabase creates a database connection.
func setupDatabase(cfg *config.Config) (*postgres.DB, error) {
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return nil, err
	}

	log.Info().
		Str("host", cfg.Database.Host).
		Int("port", cfg.Database.Port).
		Str("database", cfg.Database.Name).
		Msg("Database connection established")

	return db, nil
}

// closeDatabase closes the database connection.
func closeDatabase(db *postgres.DB) {
	if err := db.Close(); err != nil {
		log.Warn().Err(err).Msg("Failed to close database connection")
	}
}

// setupRedis creates a Redis connection (optional - graceful degradation).
func setupRedis(cfg *config.Config) (*redisinfra.Client, *redisinfra.UOMCache) {
	redisClient, err := redisinfra.NewClient(&cfg.Redis)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to Redis, continuing without cache")
		return nil, nil
	}

	uomCache := redisinfra.NewUOMCache(redisClient)
	log.Info().
		Str("host", cfg.Redis.Host).
		Int("port", cfg.Redis.Port).
		Msg("Redis connection established")

	return redisClient, uomCache
}

// closeRedis closes the Redis connection.
func closeRedis(client *redisinfra.Client) {
	if err := client.Close(); err != nil {
		log.Warn().Err(err).Msg("Failed to close Redis connection")
	}
}

// startServers starts the gRPC and HTTP servers and handles graceful shutdown.
func startServers(ctx context.Context, cfg *config.Config, uomHandler *grpcdelivery.UOMHandler) error {
	// Setup gRPC server
	grpcServer, err := grpcdelivery.NewServer(&cfg.Server, nil)
	if err != nil {
		return err
	}

	// Register UOM service
	financev1.RegisterUOMServiceServer(grpcServer.GRPCServer(), uomHandler)

	// Start gRPC server
	go func() {
		if err := grpcServer.Start(); err != nil {
			log.Error().Err(err).Msg("gRPC server failed")
		}
	}()

	// Start HTTP gateway
	httpServer := httpdelivery.NewServer(&cfg.Server)
	go func() {
		if err := httpServer.Start(ctx); err != nil {
			log.Warn().Err(err).Msg("HTTP server stopped")
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down servers...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Stop HTTP server
	if err := httpServer.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	// Stop gRPC server
	grpcServer.Stop()

	log.Info().Msg("Server shutdown complete")
	return nil
}
