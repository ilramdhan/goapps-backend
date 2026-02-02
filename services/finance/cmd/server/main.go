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
	httpdelivery "github.com/mutugading/goapps-backend/services/finance/internal/delivery/http"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	redisinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/redis"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/tracing"
)

func main() {
	// Setup logger
	zerolog.TimeFieldFormat = time.RFC3339
	if os.Getenv("APP_ENV") == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	log.Info().
		Str("service", cfg.App.Name).
		Str("version", cfg.App.Version).
		Str("environment", cfg.App.Env).
		Msg("Starting finance service")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup tracing
	tracingProvider, err := tracing.NewProvider(ctx, &cfg.Tracing, cfg.App.Name, cfg.App.Version)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to setup tracing, continuing without it")
	} else if tracingProvider != nil {
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = tracingProvider.Shutdown(shutdownCtx)
		}()
	}

	// Setup database
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer db.Close()

	log.Info().
		Str("host", cfg.Database.Host).
		Int("port", cfg.Database.Port).
		Str("database", cfg.Database.Name).
		Msg("Database connection established")

	// Setup Redis (optional - graceful degradation)
	var redisClient *redisinfra.Client
	var uomCache *redisinfra.UOMCache

	redisClient, err = redisinfra.NewClient(&cfg.Redis)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to Redis, continuing without cache")
	} else {
		defer redisClient.Close()
		uomCache = redisinfra.NewUOMCache(redisClient)
		log.Info().
			Str("host", cfg.Redis.Host).
			Int("port", cfg.Redis.Port).
			Msg("Redis connection established")
	}

	// Setup repository
	uomRepo := postgres.NewUOMRepository(db)

	// Setup gRPC handler
	uomHandler, err := grpcdelivery.NewUOMHandler(uomRepo, uomCache)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create UOM handler")
	}

	// Setup gRPC server
	grpcServer, err := grpcdelivery.NewServer(&cfg.Server, db)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create gRPC server")
	}

	// Register UOM service
	financev1.RegisterUOMServiceServer(grpcServer.GRPCServer(), uomHandler)

	// Start gRPC server
	go func() {
		if err := grpcServer.Start(); err != nil {
			log.Fatal().Err(err).Msg("gRPC server failed")
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
}
