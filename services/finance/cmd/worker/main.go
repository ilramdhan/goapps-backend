// Package main is the entry point for the finance worker service.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costbulkimport"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costproductapplicableparam"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/oraclesync"
	apprmcost "github.com/mutugading/goapps-backend/services/finance/internal/application/rmcost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamclient"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
	workerinternal "github.com/mutugading/goapps-backend/services/finance/internal/worker"
)

func main() {
	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("Worker failed")
	}
}

func run() error { //nolint:gocognit,gocyclo // linear setup function
	setupLogger()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log.Info().
		Str("service", cfg.App.Name+"-worker").
		Str("version", cfg.App.Version).
		Str("environment", cfg.App.Env).
		Msg("Starting finance worker")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup PostgreSQL.
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return err
	}
	defer closeResource("database", db)

	log.Info().
		Str("host", cfg.Database.Host).
		Int("port", cfg.Database.Port).
		Msg("Database connected")

	// Setup Oracle (optional - graceful degradation; RM cost jobs don't need it).
	oracleClient, err := oracle.NewClient(cfg.Oracle, log.Logger)
	if err != nil {
		log.Warn().Err(err).Msg("Oracle unavailable; oracle_sync jobs will be skipped")
		oracleClient = nil
	} else {
		defer closeResource("oracle", oracleClient)
		log.Info().
			Str("host", cfg.Oracle.Host).
			Int("port", cfg.Oracle.Port).
			Msg("Oracle connected")
	}

	// Setup RabbitMQ.
	rmqConn, err := rabbitmq.NewConnection(cfg.RabbitMQ, log.Logger)
	if err != nil {
		return err
	}
	defer closeResource("rabbitmq", rmqConn)

	// Create repositories.
	jobRepo := postgres.NewJobRepository(db)
	var oracleRepo *oracle.ItemConsStockPORepository
	if oracleClient != nil {
		oracleRepo = oracle.NewItemConsStockPORepository(oracleClient)
	}
	syncDataRepo := postgres.NewSyncDataRepository(db)
	rmGroupRepo := postgres.NewRMGroupRepository(db)
	rmCostRepo := postgres.NewRMCostRepository(db)
	rmCostDetailRepo := postgres.NewRMCostDetailRepository(db)

	// RabbitMQ publisher (also used by sync handler to chain-trigger rm cost).
	rmqPublisher := rabbitmq.NewPublisher(rmqConn, log.Logger)
	rmqJobPub := rabbitmq.NewJobPublisherAdapter(rmqPublisher, log.Logger)

	// Create sync handler with chain publisher (only when Oracle is available).
	var syncHandler *oraclesync.SyncHandler
	if oracleRepo != nil {
		syncHandler = oraclesync.NewSyncHandler(jobRepo, oracleRepo, syncDataRepo, log.Logger).
			WithChainPublisher(rmqJobPub)
	}

	// Create rm cost calculation handler (V2 engine).
	rmCostCalcV2 := apprmcost.NewCalculateHandlerV2(rmGroupRepo, rmCostRepo, rmCostDetailRepo, syncDataRepo, syncDataRepo)
	rmCostExec := apprmcost.NewExecuteHandlerV2(jobRepo, rmGroupRepo, rmCostCalcV2, log.Logger)

	storageSvc := setupStorage(cfg)
	iamNotif, closeIAM := setupIAMClient(cfg)
	if closeIAM != nil {
		defer closeIAM()
	}

	// Create rm cost export handler.
	rmCostExportHandler := workerinternal.NewRMCostExportHandler(jobRepo, rmCostRepo, rmCostDetailRepo, storageSvc, iamNotif, log.Logger)

	// Create costing import handler.
	costImportJobRepo := postgres.NewCostImportJobRepository(db)
	cptRepo := postgres.NewCostProductTypeRepository(db)
	cpmRepo := postgres.NewCostProductMasterRepository(db)
	cappRepo := postgres.NewCostProductParameterRepository(db)
	cppRepo := postgres.NewCostProductParameterRepository(db)
	costRouteRepo := postgres.NewCostRouteRepository(db)
	cpmImportHandler := costproductmaster.NewAsyncImportHandler(cpmRepo, cptRepo, costImportJobRepo)
	cappImportHandler := costproductapplicableparam.NewAsyncImportHandler(cappRepo, costImportJobRepo)
	cppImportHandler := costproductparameter.NewAsyncImportHandler(cppRepo, costImportJobRepo)
	bulkImportHandler := costbulkimport.NewBulkImportHandler(
		costImportJobRepo, cpmRepo, cppRepo, costRouteRepo, cptRepo, storageSvc, log.Logger,
	)
	bulkExportHandler := costbulkimport.NewExportHandler(
		cpmRepo, cppRepo, cptRepo, costRouteRepo, costImportJobRepo, storageSvc, log.Logger,
	)
	paramOnlyImportHandler := costbulkimport.NewParamOnlyImportHandler(
		costImportJobRepo, cpmRepo, cppRepo, storageSvc, log.Logger,
	)
	costingImportHandler := workerinternal.NewCostingImportHandler(
		costImportJobRepo, storageSvc,
		cpmImportHandler, cappImportHandler, cppImportHandler,
		bulkImportHandler, bulkExportHandler,
		paramOnlyImportHandler,
		iamNotif, log.Logger,
	)

	consumers := buildConsumers(rmqConn, syncHandler, rmCostExec, rmCostExportHandler, costingImportHandler)

	go watchConnection(ctx, rmqConn)

	if runErr := runConsumers(ctx, cancel, consumers); runErr != nil {
		return runErr
	}

	log.Info().Msg("Waiting for in-flight jobs to complete...")
	time.Sleep(5 * time.Second)
	log.Info().Msg("Worker shutdown complete")
	return nil
}

// buildConsumers wires the four rabbitmq consumers (oracle_sync, rm_cost_calc,
// rm_cost_export, costing_import) with their respective message handlers.
func buildConsumers(
	rmqConn *rabbitmq.Connection,
	syncHandler *oraclesync.SyncHandler,
	rmCostExec *apprmcost.ExecuteHandlerV2,
	rmCostExportHandler *workerinternal.RMCostExportHandler,
	costingImportHandler *workerinternal.CostingImportHandler,
) []*rabbitmq.Consumer {
	syncMsgHandler := func(ctx context.Context, msg rabbitmq.JobMessage) error {
		return runOracleSyncJob(ctx, syncHandler, msg)
	}
	rmCostMsgHandler := func(ctx context.Context, msg rabbitmq.JobMessage) error {
		return runRMCostCalcJob(ctx, rmCostExec, msg)
	}
	rmCostExportMsgHandler := func(ctx context.Context, msg rabbitmq.JobMessage) error {
		return rmCostExportHandler.Handle(ctx, msg)
	}
	costingImportMsgHandler := func(ctx context.Context, msg rabbitmq.JobMessage) error {
		return costingImportHandler.Handle(ctx, msg)
	}
	return []*rabbitmq.Consumer{
		rabbitmq.NewConsumer(rmqConn, rabbitmq.QueueOracleSync, syncMsgHandler, log.Logger),
		rabbitmq.NewConsumer(rmqConn, rabbitmq.QueueRMCostCalc, rmCostMsgHandler, log.Logger),
		rabbitmq.NewConsumer(rmqConn, rabbitmq.QueueRMCostExport, rmCostExportMsgHandler, log.Logger),
		rabbitmq.NewConsumer(rmqConn, rabbitmq.QueueImportJob, costingImportMsgHandler, log.Logger),
	}
}

// runOracleSyncJob parses + dispatches an oracle_sync message to the handler.
func runOracleSyncJob(ctx context.Context, h *oraclesync.SyncHandler, msg rabbitmq.JobMessage) error {
	if h == nil {
		log.Warn().Str("job_id", msg.JobID).Msg("Oracle sync job received but Oracle unavailable; skipping")
		return nil
	}
	jobID, parseErr := uuid.Parse(msg.JobID)
	if parseErr != nil {
		log.Error().Err(parseErr).Str("job_id", msg.JobID).Msg("Invalid job ID in message")
		return parseErr
	}
	return h.Execute(ctx, jobID)
}

// runRMCostCalcJob parses + dispatches an rm_cost_calculation message.
func runRMCostCalcJob(ctx context.Context, h *apprmcost.ExecuteHandlerV2, msg rabbitmq.JobMessage) error {
	jobID, parseErr := uuid.Parse(msg.JobID)
	if parseErr != nil {
		log.Error().Err(parseErr).Str("job_id", msg.JobID).Msg("Invalid rm cost job ID")
		return parseErr
	}
	cmd := apprmcost.ExecuteCommand{
		JobID:         jobID,
		Period:        msg.Period,
		CalculatedBy:  msg.CreatedBy,
		TriggerReason: rmcost.HistoryTriggerReason(msg.Reason),
	}
	if msg.GroupHeadID != "" {
		gid, gErr := uuid.Parse(msg.GroupHeadID)
		if gErr != nil {
			log.Error().Err(gErr).Str("group_head_id", msg.GroupHeadID).Msg("Invalid group head id in rm cost message")
			return gErr
		}
		cmd.GroupHeadID = &gid
	}
	return h.Execute(ctx, cmd)
}

// runConsumers fans out the consumers into goroutines and waits for either an
// OS shutdown signal or the first consumer error.
func runConsumers(ctx context.Context, cancel context.CancelFunc, consumers []*rabbitmq.Consumer) error {
	errCh := make(chan error, len(consumers))
	for _, c := range consumers {
		go func() { errCh <- c.Start(ctx) }()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Info().Str("signal", sig.String()).Msg("Shutdown signal received")
		cancel()
	case err := <-errCh:
		if err != nil {
			log.Error().Err(err).Msg("Consumer error")
			cancel()
			return err
		}
	}
	return nil
}

func setupLogger() {
	zerolog.TimeFieldFormat = time.RFC3339
	if os.Getenv("APP_ENV") == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}

type closer interface {
	Close() error
}

func closeResource(name string, c closer) {
	if err := c.Close(); err != nil {
		log.Warn().Err(err).Str("resource", name).Msg("Failed to close resource")
	}
}

func watchConnection(ctx context.Context, conn *rabbitmq.Connection) {
	closeCh := conn.NotifyClose()
	select {
	case <-ctx.Done():
		return
	case err := <-closeCh:
		if err != nil {
			log.Error().Err(err).Msg("RabbitMQ connection lost")
		}
	}
}

// setupStorage builds a MinIO client, falling back to nil on dial failure so
// rm_cost_export jobs gracefully fail rather than crashing the worker.
func setupStorage(cfg *config.Config) storage.Service {
	c, err := storage.NewMinIOClient(storage.Config{
		Endpoint:           cfg.Storage.Endpoint,
		AccessKey:          cfg.Storage.AccessKey,
		SecretKey:          cfg.Storage.SecretKey,
		Bucket:             cfg.Storage.Bucket,
		UseSSL:             cfg.Storage.UseSSL,
		InsecureSkipVerify: cfg.Storage.InsecureSkipVerify,
		Region:             cfg.Storage.Region,
		PublicURL:          cfg.Storage.PublicURL,
	})
	if err != nil {
		log.Warn().Err(err).Msg("MinIO unavailable; rm_cost_export jobs will fail")
		return nil
	}
	return c
}

// setupIAMClient dials IAM and returns the notification client + a deferred
// close callback. Falls back to a no-op client when dial fails so the worker
// keeps running (notifications will be silently dropped).
func setupIAMClient(cfg *config.Config) (iamclient.NotificationClient, func()) {
	cli, err := iamclient.NewClient(cfg.IAMClient.Host, cfg.IAMClient.Port, cfg.IAMClient.InternalServiceToken)
	if err != nil {
		log.Warn().Err(err).Msg("IAM client unavailable; export-ready notifications will be skipped")
		return iamclient.NewNopClient(), nil
	}
	return cli, func() { closeResource("iam-client", cli) }
}
