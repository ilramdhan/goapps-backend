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
	"github.com/mutugading/goapps-backend/services/finance/internal/application/oraclesync"
	apprmcost "github.com/mutugading/goapps-backend/services/finance/internal/application/rmcost"
	grpcdelivery "github.com/mutugading/goapps-backend/services/finance/internal/delivery/grpc"
	httpdelivery "github.com/mutugading/goapps-backend/services/finance/internal/delivery/httpdelivery"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
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

	// Setup RabbitMQ (optional - graceful degradation for publisher)
	rmqAdapter, closeRabbitMQ := setupRabbitMQ(cfg)
	defer closeRabbitMQ()

	// Wrap into explicit interface values so that when RabbitMQ is unavailable
	// the handlers receive a true nil interface (not a typed-nil pointer).
	var oracleSyncPublisher oraclesync.JobPublisher
	var rmCostPublisher apprmcost.JobPublisher
	if rmqAdapter != nil {
		oracleSyncPublisher = rmqAdapter
		rmCostPublisher = rmqAdapter
	}

	// Setup repositories
	uomRepo := postgres.NewUOMRepository(db)
	rmCategoryRepo := postgres.NewRMCategoryRepository(db)
	parameterRepo := postgres.NewParameterRepository(db)
	formulaRepo := postgres.NewFormulaRepository(db)
	uomCategoryRepo := postgres.NewUOMCategoryRepository(db)
	jobRepo := postgres.NewJobRepository(db)
	syncDataRepo := postgres.NewSyncDataRepository(db)
	rmGroupRepo := postgres.NewRMGroupRepository(db)
	rmCostRepo := postgres.NewRMCostRepository(db)

	// Setup oracle sync handlers
	triggerHandler := oraclesync.NewTriggerHandler(jobRepo, oracleSyncPublisher)
	getJobHandler := oraclesync.NewGetJobHandler(jobRepo)
	listJobsHandler := oraclesync.NewListJobsHandler(jobRepo)
	cancelJobHandler := oraclesync.NewCancelJobHandler(jobRepo)
	listDataHandler := oraclesync.NewListDataHandler(syncDataRepo)
	listPeriodsHandler := oraclesync.NewListPeriodsHandler(syncDataRepo)

	// Setup gRPC handlers
	uomHandler, err := grpcdelivery.NewUOMHandler(uomRepo, uomCategoryRepo, uomCache)
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

	formulaHandler, err := grpcdelivery.NewFormulaHandler(formulaRepo)
	if err != nil {
		return err
	}

	uomCategoryHandler, err := grpcdelivery.NewUOMCategoryHandler(uomCategoryRepo)
	if err != nil {
		return err
	}

	oracleSyncHandler, err := grpcdelivery.NewOracleSyncHandler(
		triggerHandler, getJobHandler, listJobsHandler,
		cancelJobHandler, listDataHandler, listPeriodsHandler,
	)
	if err != nil {
		return err
	}

	recalcChain := grpcdelivery.NewRecalcChain(
		jobRepo,
		rmCostPublisher,
		rmCostRepo.ListDistinctPeriods,
		syncDataRepo.GetDistinctPeriods,
	)
	rmGroupHandler, err := grpcdelivery.NewRMGroupHandler(rmGroupRepo, syncDataRepo, syncDataRepo, syncDataRepo, rmCostRepo, syncDataRepo, recalcChain)
	if err != nil {
		return err
	}

	rmCostTrigger := apprmcost.NewTriggerHandler(jobRepo, rmCostPublisher)
	rmCostCalculate := apprmcost.NewCalculateHandler(rmGroupRepo, rmCostRepo, syncDataRepo)
	rmCostGet := apprmcost.NewGetHandler(rmCostRepo)
	rmCostList := apprmcost.NewListHandler(rmCostRepo)
	rmCostHistory := apprmcost.NewHistoryHandler(rmCostRepo)
	rmCostPeriods := apprmcost.NewPeriodsHandler(rmCostRepo)
	rmCostExport := apprmcost.NewExportHandler(rmCostRepo)

	rmCostHandler, err := grpcdelivery.NewRMCostHandler(
		rmCostTrigger, rmCostCalculate, rmCostGet, rmCostList, rmCostHistory, rmCostPeriods, rmCostExport,
	)
	if err != nil {
		return err
	}

	// Setup and start servers
	return startServers(ctx, cfg, uomHandler, rmCategoryHandler, parameterHandler, formulaHandler, uomCategoryHandler, oracleSyncHandler, rmGroupHandler, rmCostHandler, tokenBlacklist)
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
func startServers(ctx context.Context, cfg *config.Config, uomHandler *grpcdelivery.UOMHandler, rmCategoryHandler *grpcdelivery.RMCategoryHandler, parameterHandler *grpcdelivery.ParameterHandler, formulaHandler *grpcdelivery.FormulaHandler, uomCategoryHandler *grpcdelivery.UOMCategoryHandler, oracleSyncHandler *grpcdelivery.OracleSyncHandler, rmGroupHandler *grpcdelivery.RMGroupHandler, rmCostHandler *grpcdelivery.RMCostHandler, tokenBlacklist *redisinfra.TokenBlacklist) error {
	// Setup gRPC server with JWT auth and token blacklist
	grpcServer, err := grpcdelivery.NewServer(&cfg.Server, nil, &cfg.JWT, tokenBlacklist)
	if err != nil {
		return err
	}

	// Register services
	financev1.RegisterUOMServiceServer(grpcServer.GRPCServer(), uomHandler)
	financev1.RegisterRMCategoryServiceServer(grpcServer.GRPCServer(), rmCategoryHandler)
	financev1.RegisterParameterServiceServer(grpcServer.GRPCServer(), parameterHandler)
	financev1.RegisterFormulaServiceServer(grpcServer.GRPCServer(), formulaHandler)
	financev1.RegisterUOMCategoryServiceServer(grpcServer.GRPCServer(), uomCategoryHandler)
	financev1.RegisterOracleSyncServiceServer(grpcServer.GRPCServer(), oracleSyncHandler)
	financev1.RegisterRMGroupServiceServer(grpcServer.GRPCServer(), rmGroupHandler)
	financev1.RegisterRMCostServiceServer(grpcServer.GRPCServer(), rmCostHandler)

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

// setupRabbitMQ creates a RabbitMQ connection and publisher (optional - graceful degradation).
// Returns a JobPublisherAdapter and a close function for graceful shutdown.
func setupRabbitMQ(cfg *config.Config) (*rabbitmq.JobPublisherAdapter, func()) {
	rmqConn, err := rabbitmq.NewConnection(cfg.RabbitMQ, log.Logger)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to RabbitMQ, sync trigger will fail")
		return nil, func() {}
	}

	publisher := rabbitmq.NewPublisher(rmqConn, log.Logger)
	adapter := rabbitmq.NewJobPublisherAdapter(publisher, log.Logger)
	closeFunc := func() {
		if closeErr := rmqConn.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close RabbitMQ connection")
		}
	}
	return adapter, closeFunc
}
