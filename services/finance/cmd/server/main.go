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
	chartdataapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dashboardapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/dashboard"
	datasourceapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/datasource"
	groupapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/group"
	jobapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/job"
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

	// Setup shared auth Redis for token blacklist (optional - graceful degradation)
	tokenBlacklist := setupAuthRedis(cfg)
	if tokenBlacklist != nil {
		defer closeAuthRedis(tokenBlacklist)
	}

	// Setup repositories
	uomRepo := postgres.NewUOMRepository(db)
	rmCategoryRepo := postgres.NewRMCategoryRepository(db)
	parameterRepo := postgres.NewParameterRepository(db)

	// BI repositories + cache
	biDashboardRepo := postgres.NewBiDashboardRepository(db)
	biGroupRepo := postgres.NewBiDashboardGroupRepository(db)
	biFactRepo := postgres.NewBiFactMetricRepository(db)
	biDataSourceRepo := postgres.NewBiDataSourceRepository(db)
	biJobRepo := postgres.NewBiJobRepository(db)
	biChartCache := redisinfra.NewChartCache(redisClient)

	// Setup gRPC handlers
	uomHandler, err := grpcdelivery.NewUOMHandler(uomRepo, uomCache)
	if err != nil {
		return err
	}

	rmCategoryHandler, err := grpcdelivery.NewRMCategoryHandler(rmCategoryRepo)
	if err != nil {
		return err
	}

	parameterHandler, err := grpcdelivery.NewParameterHandler(parameterRepo)
	if err != nil {
		return err
	}

	// BI gRPC handlers
	biDashboardHandler, err := grpcdelivery.NewBIDashboardHandler(
		dashboardapp.NewCreateHandler(biDashboardRepo),
		dashboardapp.NewGetHandler(biDashboardRepo),
		dashboardapp.NewListHandler(biDashboardRepo),
		dashboardapp.NewUpdateHandler(biDashboardRepo, biChartCache),
		dashboardapp.NewDeleteHandler(biDashboardRepo, biChartCache),
		dashboardapp.NewDuplicateHandler(biDashboardRepo),
		dashboardapp.NewSetRolesHandler(biDashboardRepo, biChartCache),
		dashboardapp.NewListAccessibleHandler(biDashboardRepo),
		groupapp.NewCreateHandler(biGroupRepo),
		groupapp.NewListHandler(biGroupRepo),
		groupapp.NewUpdateHandler(biGroupRepo),
		groupapp.NewDeleteHandler(biGroupRepo),
	)
	if err != nil {
		return err
	}

	biChartDataHandler, err := grpcdelivery.NewBIChartDataHandler(
		chartdataapp.NewGetDataHandler(biDashboardRepo, biFactRepo, biChartCache, redisinfra.HashFilters),
		chartdataapp.NewPreviewHandler(biFactRepo),
	)
	if err != nil {
		return err
	}

	biDataSourceHandler, err := grpcdelivery.NewBIDataSourceHandler(
		datasourceapp.NewListHandler(biDataSourceRepo),
		datasourceapp.NewGetDistinctsHandler(biFactRepo),
	)
	if err != nil {
		return err
	}

	biJobHandler, err := grpcdelivery.NewBIJobHandler(
		jobapp.NewListHandler(biJobRepo),
		jobapp.NewListLogsHandler(biJobRepo),
		jobapp.NewTriggerHandler(biJobRepo),
	)
	if err != nil {
		return err
	}

	// Setup and start servers
	return startServers(ctx, cfg, uomHandler, rmCategoryHandler, parameterHandler,
		biDashboardHandler, biChartDataHandler, biDataSourceHandler, biJobHandler,
		tokenBlacklist)
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

// setupAuthRedis creates a Redis connection to IAM's shared blacklist (optional).
func setupAuthRedis(cfg *config.Config) *redisinfra.TokenBlacklist {
	blacklist, err := redisinfra.NewTokenBlacklist(&cfg.AuthRedis)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to auth Redis, continuing without token blacklist")
		return nil
	}
	return blacklist
}

// closeAuthRedis closes the auth Redis connection.
func closeAuthRedis(bl *redisinfra.TokenBlacklist) {
	if err := bl.Close(); err != nil {
		log.Warn().Err(err).Msg("Failed to close auth Redis connection")
	}
}

// startServers starts the gRPC and HTTP servers and handles graceful shutdown.
func startServers(
	ctx context.Context,
	cfg *config.Config,
	uomHandler *grpcdelivery.UOMHandler,
	rmCategoryHandler *grpcdelivery.RMCategoryHandler,
	parameterHandler *grpcdelivery.ParameterHandler,
	biDashboardHandler *grpcdelivery.BIDashboardHandler,
	biChartDataHandler *grpcdelivery.BIChartDataHandler,
	biDataSourceHandler *grpcdelivery.BIDataSourceHandler,
	biJobHandler *grpcdelivery.BIJobHandler,
	tokenBlacklist *redisinfra.TokenBlacklist,
) error {
	// Setup gRPC server with JWT auth and token blacklist
	grpcServer, err := grpcdelivery.NewServer(&cfg.Server, nil, &cfg.JWT, tokenBlacklist)
	if err != nil {
		return err
	}

	// Register services
	financev1.RegisterUOMServiceServer(grpcServer.GRPCServer(), uomHandler)
	financev1.RegisterRMCategoryServiceServer(grpcServer.GRPCServer(), rmCategoryHandler)
	financev1.RegisterParameterServiceServer(grpcServer.GRPCServer(), parameterHandler)

	// BI services
	financev1.RegisterDashboardServiceServer(grpcServer.GRPCServer(), biDashboardHandler)
	financev1.RegisterChartDataServiceServer(grpcServer.GRPCServer(), biChartDataHandler)
	financev1.RegisterDataSourceServiceServer(grpcServer.GRPCServer(), biDataSourceHandler)
	financev1.RegisterBiJobServiceServer(grpcServer.GRPCServer(), biJobHandler)

	// Start gRPC server
	go func() {
		if err := grpcServer.Start(); err != nil {
			log.Error().Err(err).Msg("gRPC server failed")
		}
	}()

	// Start HTTP gateway with CORS config
	httpServer := httpdelivery.NewServer(&cfg.Server,
		httpdelivery.WithCORS(cfg.CORS.AllowedOrigins, cfg.CORS.MaxAge),
	)
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
