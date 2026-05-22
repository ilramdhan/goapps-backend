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
	"github.com/mutugading/goapps-backend/services/finance/internal/application/auditadapter"
	auditapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costauditlog"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	cppapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/oraclesync"
	apprmcost "github.com/mutugading/goapps-backend/services/finance/internal/application/rmcost"
	grpcdelivery "github.com/mutugading/goapps-backend/services/finance/internal/delivery/grpc"
	httpdelivery "github.com/mutugading/goapps-backend/services/finance/internal/delivery/httpdelivery"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
	redisinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/redis"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
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
	rmqAdapter, costJobPub, closeRabbitMQ := setupRabbitMQ(cfg)
	defer closeRabbitMQ()

	// Wrap into explicit interface values so that when RabbitMQ is unavailable
	// the handlers receive a true nil interface (not a typed-nil pointer).
	var oracleSyncPublisher oraclesync.JobPublisher
	var rmCostPublisher apprmcost.JobPublisher
	var rmCostExportPublisher apprmcost.ExportJobPublisher
	var costCalcJobTriggerPub costcalc.JobTriggerPublisher
	if rmqAdapter != nil {
		oracleSyncPublisher = rmqAdapter
		rmCostPublisher = rmqAdapter
		rmCostExportPublisher = rmqAdapter
	}
	if costJobPub != nil {
		costCalcJobTriggerPub = costJobPub
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
	rmCostDetailRepo := postgres.NewRMCostDetailRepository(db)
	rmCostInputsRepo := postgres.NewRMCostInputsRepository(db)
	// NOTE: legacy productRepo / prdRequestRepo wired to dropped tables — removed.
	// Canonical Phase B (cost_product_master, cost_product_order) wiring added in S2.8-S2.10.

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
	rmCostRequestExport := apprmcost.NewRequestExportHandler(jobRepo, rmCostExportPublisher)

	// MinIO storage — shared between RM Cost export downloads and Phase A attachments.
	var rmCostExportURL *apprmcost.GetExportURLHandler
	var storageSvc storage.Service
	if client, sErr := storage.NewMinIOClient(storage.Config{
		Endpoint:           cfg.Storage.Endpoint,
		AccessKey:          cfg.Storage.AccessKey,
		SecretKey:          cfg.Storage.SecretKey,
		Bucket:             cfg.Storage.Bucket,
		UseSSL:             cfg.Storage.UseSSL,
		InsecureSkipVerify: cfg.Storage.InsecureSkipVerify,
		Region:             cfg.Storage.Region,
		PublicURL:          cfg.Storage.PublicURL,
	}); sErr != nil {
		log.Warn().Err(sErr).Msg("MinIO unavailable; export download URLs + attachments will return 503")
	} else {
		storageSvc = client
		rmCostExportURL = apprmcost.NewGetExportURLHandler(jobRepo, client, 5*time.Minute)
	}

	editInputsHandler := apprmcost.NewEditInputsHandler(rmCostRepo, rmCostInputsRepo)
	editFixRateHandler := apprmcost.NewEditFixRateHandler(rmCostRepo, rmCostDetailRepo, rmCostInputsRepo)

	rmCostHandler, err := grpcdelivery.NewRMCostHandler(
		rmCostTrigger, rmCostCalculate, rmCostGet, rmCostList, rmCostHistory, rmCostPeriods, rmCostExport, rmCostRequestExport, rmCostExportURL,
		rmCostDetailRepo, editInputsHandler, editFixRateHandler,
	)
	if err != nil {
		return err
	}

	// Canonical Phase B (cost_*) repositories + handlers.
	costProductTypeRepo := postgres.NewCostProductTypeRepository(db)
	costRmTypeRepo := postgres.NewCostRmTypeRepository(db)
	costErpRepo := postgres.NewCostErpRepository(db)
	costProductMasterRepo := postgres.NewCostProductMasterRepository(db)
	costRouteRepo := postgres.NewCostRouteRepository(db)
	// Canonical Phase A (PRD §7.1) repositories.
	costRequestTypeRepo := postgres.NewCostRequestTypeRepository(db)
	costPaperTubeTypeRepo := postgres.NewCostPaperTubeTypeRepository(db)
	costProductRequestRepo := postgres.NewCostProductRequestRepository(db)
	costRequestCommentRepo := postgres.NewCostRequestCommentRepository(db)
	costAttachmentRepo := postgres.NewCostAttachmentRepository(db)
	costRoutingRuleRepo := postgres.NewCostRoutingRuleRepository(db)
	costAuditLogRepo := postgres.NewCostAuditLogRepository(db)
	costNotificationRepo := postgres.NewCostNotificationRepository(db)
	costProductParameterRepo := postgres.NewCostProductParameterRepository(db)

	costProductTypeHandler, err := grpcdelivery.NewCostProductTypeHandler(costProductTypeRepo)
	if err != nil {
		return err
	}
	costRmTypeHandler, err := grpcdelivery.NewCostRmTypeHandler(costRmTypeRepo)
	if err != nil {
		return err
	}
	costErpHandler, err := grpcdelivery.NewCostErpHandler(costErpRepo)
	if err != nil {
		return err
	}
	costProductMasterHandler, err := grpcdelivery.NewCostProductMasterHandler(costProductMasterRepo)
	if err != nil {
		return err
	}
	costRouteHandler, err := grpcdelivery.NewCostRouteHandler(costRouteRepo, costProductRequestRepo)
	if err != nil {
		return err
	}
	costRequestTypeHandler, err := grpcdelivery.NewCostRequestTypeHandler(costRequestTypeRepo)
	if err != nil {
		return err
	}
	costPaperTubeTypeHandler, err := grpcdelivery.NewCostPaperTubeTypeHandler(costPaperTubeTypeRepo)
	if err != nil {
		return err
	}
	// Audit emitter — appends CAL_ rows from request state transitions (S7.5).
	auditEmitter := auditadapter.NewCprEmitter(auditapp.NewEmitter(costAuditLogRepo))
	costProductRequestHandler, err := grpcdelivery.NewCostProductRequestHandler(costProductRequestRepo, costRouteRepo, auditEmitter)
	if err != nil {
		return err
	}
	costRequestCommentHandler, err := grpcdelivery.NewCostRequestCommentHandler(costRequestCommentRepo)
	if err != nil {
		return err
	}
	costAttachmentHandler, err := grpcdelivery.NewCostAttachmentHandler(costAttachmentRepo, storageSvc)
	if err != nil {
		return err
	}
	costRoutingRuleHandler, err := grpcdelivery.NewCostRoutingRuleHandler(costRoutingRuleRepo)
	if err != nil {
		return err
	}
	costAuditLogHandler, err := grpcdelivery.NewCostAuditLogHandler(costAuditLogRepo)
	if err != nil {
		return err
	}
	costNotificationHandler, err := grpcdelivery.NewCostNotificationHandler(costNotificationRepo)
	if err != nil {
		return err
	}
	costProductParameterApp := cppapp.New(costProductParameterRepo)
	costProductParameterHandler := grpcdelivery.NewCostProductParameterHandler(costProductParameterApp)

	// S8b: real CostCalcService wiring. Service holds 5 repos + loader + evaluator
	// cache; 11 application handlers wrap individual use cases. Audit emitter is
	// nil for now (cost_audit_log integration lands in S8c orchestrator).
	calcJobRepo := postgres.NewCostCalcJobRepository(db)
	calcChunkRepo := postgres.NewCostCalcChunkRepository(db)
	calcJobProductRepo := postgres.NewCostCalcJobProductRepository(db)
	costResultRepo := postgres.NewCostResultRepository(db)
	costAuditHistoryRepo := postgres.NewCostAuditHistoryRepository(db)
	calcEvalCache := evaluator.NewCache()
	calcLoader := costcalc.NewProductLoader(db.DB)
	calcSvc := costcalc.NewService(
		calcJobRepo, calcChunkRepo, calcJobProductRepo, costResultRepo, costAuditHistoryRepo,
		calcLoader, calcEvalCache, nil, costCalcJobTriggerPub,
	)
	costCalcHandler := grpcdelivery.NewCostCalcHandler(
		calcSvc,
		costcalc.NewTriggerJobHandler(calcSvc),
		costcalc.NewGetJobHandler(calcSvc),
		costcalc.NewListJobsHandler(calcSvc),
		costcalc.NewListChunksHandler(calcSvc),
		costcalc.NewListJobProductsHandler(calcSvc),
		costcalc.NewCancelJobHandler(calcSvc),
		costcalc.NewGetCostResultHandler(calcSvc),
		costcalc.NewGetCostBreakdownHandler(calcSvc),
		costcalc.NewListCostHistoryHandler(calcSvc),
		costcalc.NewVerifyCostHandler(calcSvc),
		costcalc.NewApproveCostHandler(calcSvc),
	)

	// Setup and start servers
	return startServers(ctx, cfg,
		uomHandler, rmCategoryHandler, parameterHandler, formulaHandler, uomCategoryHandler,
		oracleSyncHandler, rmGroupHandler, rmCostHandler,
		costProductTypeHandler, costRmTypeHandler, costErpHandler, costProductMasterHandler, costRouteHandler,
		costRequestTypeHandler, costPaperTubeTypeHandler, costProductRequestHandler,
		costRequestCommentHandler, costAttachmentHandler,
		costRoutingRuleHandler, costAuditLogHandler, costNotificationHandler,
		costProductParameterHandler,
		costCalcHandler,
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
func startServers(ctx context.Context, cfg *config.Config,
	uomHandler *grpcdelivery.UOMHandler,
	rmCategoryHandler *grpcdelivery.RMCategoryHandler,
	parameterHandler *grpcdelivery.ParameterHandler,
	formulaHandler *grpcdelivery.FormulaHandler,
	uomCategoryHandler *grpcdelivery.UOMCategoryHandler,
	oracleSyncHandler *grpcdelivery.OracleSyncHandler,
	rmGroupHandler *grpcdelivery.RMGroupHandler,
	rmCostHandler *grpcdelivery.RMCostHandler,
	costProductTypeHandler *grpcdelivery.CostProductTypeHandler,
	costRmTypeHandler *grpcdelivery.CostRmTypeHandler,
	costErpHandler *grpcdelivery.CostErpHandler,
	costProductMasterHandler *grpcdelivery.CostProductMasterHandler,
	costRouteHandler *grpcdelivery.CostRouteHandler,
	costRequestTypeHandler *grpcdelivery.CostRequestTypeHandler,
	costPaperTubeTypeHandler *grpcdelivery.CostPaperTubeTypeHandler,
	costProductRequestHandler *grpcdelivery.CostProductRequestHandler,
	costRequestCommentHandler *grpcdelivery.CostRequestCommentHandler,
	costAttachmentHandler *grpcdelivery.CostAttachmentHandler,
	costRoutingRuleHandler *grpcdelivery.CostRoutingRuleHandler,
	costAuditLogHandler *grpcdelivery.CostAuditLogHandler,
	costNotificationHandler *grpcdelivery.CostNotificationHandler,
	costProductParameterHandler *grpcdelivery.CostProductParameterHandler,
	costCalcHandler *grpcdelivery.CostCalcHandler,
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
	financev1.RegisterFormulaServiceServer(grpcServer.GRPCServer(), formulaHandler)
	financev1.RegisterUOMCategoryServiceServer(grpcServer.GRPCServer(), uomCategoryHandler)
	financev1.RegisterOracleSyncServiceServer(grpcServer.GRPCServer(), oracleSyncHandler)
	financev1.RegisterRMGroupServiceServer(grpcServer.GRPCServer(), rmGroupHandler)
	financev1.RegisterRMCostServiceServer(grpcServer.GRPCServer(), rmCostHandler)
	// Canonical Phase B services (PRD §7.2-§7.3).
	financev1.RegisterCostProductTypeServiceServer(grpcServer.GRPCServer(), costProductTypeHandler)
	financev1.RegisterCostRmTypeServiceServer(grpcServer.GRPCServer(), costRmTypeHandler)
	financev1.RegisterCostErpLookupServiceServer(grpcServer.GRPCServer(), costErpHandler)
	financev1.RegisterCostProductMasterServiceServer(grpcServer.GRPCServer(), costProductMasterHandler)
	financev1.RegisterCostRouteServiceServer(grpcServer.GRPCServer(), costRouteHandler)
	// Canonical Phase A services (PRD §7.1).
	financev1.RegisterCostRequestTypeServiceServer(grpcServer.GRPCServer(), costRequestTypeHandler)
	financev1.RegisterCostPaperTubeTypeServiceServer(grpcServer.GRPCServer(), costPaperTubeTypeHandler)
	financev1.RegisterCostProductRequestServiceServer(grpcServer.GRPCServer(), costProductRequestHandler)
	financev1.RegisterCostRequestCommentServiceServer(grpcServer.GRPCServer(), costRequestCommentHandler)
	financev1.RegisterCostAttachmentServiceServer(grpcServer.GRPCServer(), costAttachmentHandler)
	financev1.RegisterCostRoutingRuleServiceServer(grpcServer.GRPCServer(), costRoutingRuleHandler)
	financev1.RegisterCostAuditLogServiceServer(grpcServer.GRPCServer(), costAuditLogHandler)
	financev1.RegisterCostNotificationServiceServer(grpcServer.GRPCServer(), costNotificationHandler)
	financev1.RegisterCostProductParameterServiceServer(grpcServer.GRPCServer(), costProductParameterHandler)
	// S8a foundation: CostCalcService stub.
	financev1.RegisterCostCalcServiceServer(grpcServer.GRPCServer(), costCalcHandler)

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

// setupRabbitMQ creates a RabbitMQ connection and publishers (optional -
// graceful degradation). Returns the job-publisher adapter (oracle sync / RM
// cost), the cost-calc job-trigger publisher (orchestrator hand-off), and a
// close function for graceful shutdown.
func setupRabbitMQ(cfg *config.Config) (*rabbitmq.JobPublisherAdapter, *rabbitmq.CostJobPublisher, func()) {
	rmqConn, err := rabbitmq.NewConnection(cfg.RabbitMQ, log.Logger)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to RabbitMQ, sync trigger will fail")
		return nil, nil, func() {}
	}

	publisher := rabbitmq.NewPublisher(rmqConn, log.Logger)
	adapter := rabbitmq.NewJobPublisherAdapter(publisher, log.Logger)

	costPub, costErr := rabbitmq.NewCostJobPublisher(rmqConn, log.Logger)
	if costErr != nil {
		log.Warn().Err(costErr).Msg("Failed to init cost job publisher; multi-product calc scopes will fail")
		costPub = nil
	}

	closeFunc := func() {
		if closeErr := rmqConn.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close RabbitMQ connection")
		}
	}
	return adapter, costPub, closeFunc
}
