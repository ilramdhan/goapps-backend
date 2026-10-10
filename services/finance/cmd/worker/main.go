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

	auditapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costauditlog"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costbulkimport"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costimportetl"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costproductapplicableparam"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costproductparameter"
	erpapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	appmbhead "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbsourceautofill"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/oraclesync"
	apprmcost "github.com/mutugading/goapps-backend/services/finance/internal/application/rmcost"
	erpdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamclient"
	erpmetrics "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/metrics"
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
	rmqConn, err := rabbitmq.NewConnectionWithRetry(cfg.RabbitMQ, log.Logger, 3)
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
	lookupMasterRepo := postgres.NewLookupMasterRepository(db)
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
	// mbSpinRepo resolves cpp_value_mb_spin_id for MB_SPIN lookup parameters
	// during the CPP Excel import, mirroring the interactive save path's
	// resolution (see costproductparameter.Handlers wiring in cmd/server).
	mbSpinRepo := postgres.NewMBSpinRepository(db)
	cpmImportHandler := costproductmaster.NewAsyncImportHandler(cpmRepo, cptRepo, costImportJobRepo).
		WithMBSourceAutoFill(mbsourceautofill.New( // best-effort shade-driven MB_SP_CODE/MB_SP_DYE fill after each committed batch
			postgres.NewMBSourceAutoFillStore(db),
			mbsource.NewResolver(
				mbsource.NewMBSpinProvider(mbSpinRepo, postgres.NewParameterRepository(db)),
				mbsource.NewSuperbaProvider(postgres.NewSuperbaCostSpRepository(db)),
			),
			postgres.NewParameterRepository(db),
		))
	cappImportHandler := costproductapplicableparam.NewAsyncImportHandler(cappRepo, costImportJobRepo)
	cppImportHandler := costproductparameter.NewAsyncImportHandler(cppRepo, costImportJobRepo, mbSpinRepo).
		WithOilGroupPolicy(postgres.NewOilGroupPolicyRepository(db)) // oil-cost-rm-group: validate OIL_NAME / default on blank
	bulkExportHandler := costbulkimport.NewExportHandler(
		cpmRepo, cppRepo, cptRepo, costRouteRepo, costImportJobRepo, storageSvc, log.Logger,
	)
	stagingRepo := postgres.NewCostImportStagingRepository(db)
	etlImportHandler := costimportetl.NewHandler(costImportJobRepo, stagingRepo, storageSvc, lookupMasterRepo, log.Logger)
	costingImportHandler := workerinternal.NewCostingImportHandler(
		costImportJobRepo, storageSvc,
		cpmImportHandler, cappImportHandler, cppImportHandler,
		etlImportHandler, bulkExportHandler,
		iamNotif, log.Logger,
	)

	// Product cost sheet export. The handler only needs the route cost sheet
	// query, so the costcalc service is built with the two collaborators that
	// query actually touches (result repo + product/route loader); the trigger
	// publisher is nil because the worker never queues calc jobs.
	costSheetLoader := costcalc.NewProductLoader(db.DB)
	var costSheetSvcOpts []costcalc.ServiceOption
	if rmRateLoader, ok := costSheetLoader.(costcalc.RMRateOrderLoader); ok {
		costSheetSvcOpts = append(costSheetSvcOpts, costcalc.WithRMRateOrderLoader(rmRateLoader))
	}
	if rmLandedLoader, ok := costSheetLoader.(costcalc.RMLandedOrderLoader); ok {
		costSheetSvcOpts = append(costSheetSvcOpts, costcalc.WithRMLandedOrderLoader(rmLandedLoader))
	}
	calcSvc := costcalc.NewService(
		postgres.NewCostCalcJobRepository(db),
		postgres.NewCostCalcChunkRepository(db),
		postgres.NewCostCalcJobProductRepository(db),
		postgres.NewCostResultRepository(db),
		postgres.NewCostAuditHistoryRepository(db),
		costSheetLoader,
		evaluator.NewCache(),
		nil,
		nil,
		costSheetSvcOpts...,
	)
	costSheetExportHandler := workerinternal.NewCostSheetExportHandler(
		jobRepo,
		costcalc.NewGetRouteCostSheetHandler(calcSvc),
		storageSvc,
		iamNotif,
		log.Logger,
		"",
	)

	// Bulk MB Head Regenerate (Phase C). Reuses the SAME application-layer handlers
	// the ordinary single-head Submit/Validate RPCs use (built here from the same
	// repos, not shared instances — the worker and gRPC server are separate
	// processes) so a bulk-triggered transition behaves identically to a manual one,
	// including the [G.5] composition-sum gate on submit/validate.
	mbCompositionRepo := postgres.NewMBCompositionRepository(db)
	mbHeadRepo := postgres.NewMBHeadRepository(db, mbCompositionRepo)
	mbParamRepo := postgres.NewMBParamRepository(db)
	mbBulkTransitionHandler := workerinternal.NewMBBulkTransitionHandler(
		jobRepo,
		appmbhead.NewForceUnvalidateHandler(mbHeadRepo),
		appmbhead.NewSubmitHandlerWithComposition(mbHeadRepo, mbCompositionRepo),
		appmbhead.NewValidateHandlerWithComposition(mbHeadRepo, mbParamRepo, mbCompositionRepo),
		log.Logger,
	)

	// Bulk Edit Product Params (F4, B4). cppRepo is the SAME
	// CostProductParameterRepository instance built above for the costing
	// import handlers — it already implements cpp.Repository.ApplyBulkOperations.
	productParamBulkHandler := workerinternal.NewProductParamBulkHandler(jobRepo, cppRepo, log.Logger)

	// ERP master replica sync (P0-T15/T15b). Read-only on Oracle: the reader
	// only sees the ReadOnlyGuard-wrapped querier. Nil when Oracle is absent,
	// in which case erp_master_sync messages are acked and skipped.
	var erpMasterSync *erpapp.MasterSyncHandler
	if oracleClient != nil {
		erpMasterRepo := postgres.NewErpMasterRepository(db)
		erpGradeApply := erpapp.NewGradeGroupApplyHandler(
			erpMasterRepo, auditapp.NewEmitter(postgres.NewCostAuditLogRepository(db)),
		)
		erpMasterSync = erpapp.NewMasterSyncHandler(
			oracle.NewErpMasterReader(oracleClient.ReadOnly()), erpMasterRepo, erpGradeApply,
		)
	}

	erpExec, stopErpScheduler := buildErpIntegrationExecutor(ctx, cfg, db, jobRepo, oracleClient, rmqJobPub)
	defer stopErpScheduler()

	consumers := buildConsumers(
		rmqConn, syncHandler, rmCostExec, rmCostExportHandler, costingImportHandler, costSheetExportHandler,
		mbBulkTransitionHandler,
		productParamBulkHandler,
		erpMasterSync,
		erpExec,
		cfg.RabbitMQ.ExportWorkerConcurrency,
	)

	go watchConnection(ctx, rmqConn)

	if runErr := runConsumers(ctx, cancel, consumers); runErr != nil {
		return runErr
	}

	log.Info().Msg("Waiting for in-flight jobs to complete...")
	time.Sleep(5 * time.Second)
	log.Info().Msg("Worker shutdown complete")
	return nil
}

// buildConsumers wires the five rabbitmq consumers (oracle_sync, rm_cost_calc,
// rm_cost_export, costing_import, product_cost_sheet_export) with their
// respective message handlers.
//
// Concurrency: only product_cost_sheet_export runs with a bounded worker pool
// (exportConcurrency, via NewConcurrentConsumer on its own dedicated AMQP
// channel/QoS). Every other queue stays on rabbitmq.NewConsumer (strictly
// sequential, sharing the connection's default channel at
// rabbitmq.prefetch_count):
//   - oracle_sync / rm_cost_calc: period-dependent recalculation jobs whose
//     handlers were not verified safe under concurrent execution — running
//     two calculations for overlapping periods/groups at once risks
//     interleaved writes to the same landed-cost rows. Left sequential.
//   - rm_cost_export: single-file export, no batch/child fan-out, so there is
//     no 200-jobs-at-once workload to speed up; left sequential rather than
//     changing behavior with no benefit.
//   - costing_import: bulk import handlers stage rows and run set-based ETL;
//     concurrent imports were not audited for shared staging-table/tx
//     assumptions. Left sequential (conservative default per task scope).
//   - product_cost_sheet_export: verified concurrency-safe — job state
//     transitions go through pgx (concurrency-safe pool), the evaluator
//     cache is guarded by sync.RWMutex, IncrementChildProgress is a single
//     atomic UPDATE ... RETURNING, and each delivery's workbook/temp file is
//     locally scoped (no shared writer or package-level mutable state). This
//     is also the one queue where batch parents fan out 200+ child jobs, so
//     it is the only one with a real sequential-processing bottleneck.
//   - mb_bulk_transition: each child runs a full mbhead.Entity workflow
//     transition (force_unvalidate/submit/validate) against ITS OWN mbh_id, so
//     children never race each other on the same row — but ValidateHandler's
//     underlying repo call was not audited for concurrent-write safety across
//     DIFFERENT rows sharing downstream tables (cost_product_master/
//     cost_route_*), so this is left sequential as a conservative default,
//     same reasoning as costing_import.
func buildConsumers(
	rmqConn *rabbitmq.Connection,
	syncHandler *oraclesync.SyncHandler,
	rmCostExec *apprmcost.ExecuteHandlerV2,
	rmCostExportHandler *workerinternal.RMCostExportHandler,
	costingImportHandler *workerinternal.CostingImportHandler,
	costSheetExportHandler *workerinternal.CostSheetExportHandler,
	mbBulkTransitionHandler *workerinternal.MBBulkTransitionHandler,
	productParamBulkHandler *workerinternal.ProductParamBulkHandler,
	erpMasterSync *erpapp.MasterSyncHandler,
	erpIntegration *erpapp.JobExecutor,
	exportConcurrency int,
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
	costSheetExportMsgHandler := func(ctx context.Context, msg rabbitmq.JobMessage) error {
		return costSheetExportHandler.Handle(ctx, msg)
	}
	mbBulkTransitionMsgHandler := func(ctx context.Context, msg rabbitmq.JobMessage) error {
		return mbBulkTransitionHandler.Handle(ctx, msg)
	}
	productParamBulkMsgHandler := func(ctx context.Context, msg rabbitmq.JobMessage) error {
		return productParamBulkHandler.Handle(ctx, msg)
	}
	erpMasterSyncMsgHandler := func(ctx context.Context, msg rabbitmq.JobMessage) error {
		return runErpMasterSyncJob(ctx, erpMasterSync, msg)
	}
	erpIntegrationMsgHandler := func(ctx context.Context, msg rabbitmq.JobMessage) error {
		return runErpIntegrationJob(ctx, erpIntegration, msg)
	}
	return []*rabbitmq.Consumer{
		rabbitmq.NewConsumer(rmqConn, rabbitmq.QueueOracleSync, syncMsgHandler, log.Logger),
		rabbitmq.NewConsumer(rmqConn, rabbitmq.QueueRMCostCalc, rmCostMsgHandler, log.Logger),
		rabbitmq.NewConsumer(rmqConn, rabbitmq.QueueRMCostExport, rmCostExportMsgHandler, log.Logger),
		rabbitmq.NewConsumer(rmqConn, rabbitmq.QueueImportJob, costingImportMsgHandler, log.Logger),
		rabbitmq.NewConsumer(rmqConn, rabbitmq.QueueMBBulkTransition, mbBulkTransitionMsgHandler, log.Logger),
		// product_param_bulk: mirrors mb_bulk_transition's conservative default —
		// ApplyBulkOperations has not been audited for concurrent-write safety
		// across children of the same batch (e.g. two children referencing the
		// same lookup_fill_group_code trigger param). Left sequential.
		rabbitmq.NewConsumer(rmqConn, rabbitmq.QueueProductParamBulk, productParamBulkMsgHandler, log.Logger),
		// erp_master_sync: full-table upserts of the same replica tables — must
		// never overlap, so strictly sequential.
		rabbitmq.NewConsumer(rmqConn, rabbitmq.QueueErpMasterSync, erpMasterSyncMsgHandler, log.Logger),
		// erp_integration: batch steps run under the G11 per-batch advisory
		// lock; design §10 fixes concurrency at 1, so strictly sequential.
		rabbitmq.NewConsumer(rmqConn, rabbitmq.QueueErpIntegration, erpIntegrationMsgHandler, log.Logger),
		rabbitmq.NewConcurrentConsumer(
			rmqConn, rabbitmq.QueueProductCostSheetExport, costSheetExportMsgHandler, log.Logger, exportConcurrency,
		),
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

// runErpMasterSyncJob dispatches an erp_master_sync message. Subtype is ""
// (all), om_item, om_grade or apply_grade_groups.
func runErpMasterSyncJob(ctx context.Context, h *erpapp.MasterSyncHandler, msg rabbitmq.JobMessage) error {
	if h == nil {
		log.Warn().Str("job_id", msg.JobID).Msg("ERP master sync job received but Oracle unavailable; skipping")
		return nil
	}
	res, err := h.Handle(ctx, msg.Subtype, msg.CreatedBy)
	if err != nil {
		log.Error().Err(err).Str("job_id", msg.JobID).Str("subtype", msg.Subtype).Msg("ERP master sync failed")
		return err
	}
	ev := log.Info().Str("job_id", msg.JobID).Str("subtype", msg.Subtype)
	if res.Items != nil {
		ev = ev.Interface("items", *res.Items)
	}
	if res.Grades != nil {
		ev = ev.Interface("grades", *res.Grades)
	}
	if res.GradeGroups != nil {
		ev = ev.Interface("grade_groups", *res.GradeGroups)
	}
	ev.Msg("ERP master sync completed")
	return nil
}

// buildErpIntegrationExecutor wires the erp_integration batch-step executor
// (plan-04 P3-T4). Oracle is reached only through the ReadOnlyGuard querier
// (SELECT only). Without Oracle (or with an invalid demand_source) the
// load_demand step fails its job; coverage and derive are PG-only and still
// run (plan-05 P4-T4: derive reads the active rule set in one snapshot).
//
// It also wires the W2 steps (plan-06 P5-T5: adj_execute, lock_batch) and
// the scheduled read/compute chain (P5-T11). The returned stop func stops the
// scheduler.
func buildErpIntegrationExecutor(ctx context.Context, cfg *config.Config, db *postgres.DB, jobRepo *postgres.JobRepository,
	oracleClient *oracle.Client, publisher *rabbitmq.JobPublisherAdapter,
) (*erpapp.JobExecutor, func()) {
	runner := postgres.NewErpBatchTxRunner(db)
	var load *erpapp.LoadDemandStep
	// legacyReader stays a true nil interface without Oracle, so attr_backfill
	// jobs fail closed (ErrAttrBackfillNotConfigured). FG_PRD_PER_DAY is not
	// read (WithPrdPerDay unset: column unconfirmed).
	var legacyReader erpdomain.LegacyStdReader
	// prober stays a true nil interface without Oracle, so validate reports
	// V-10 as an error (posted-head probe not configured: fail closed).
	var prober erpdomain.ErpAdjHeadProber
	if oracleClient != nil {
		legacyReader = oracle.NewLegacyStdReader(oracleClient.ReadOnly())
		prober = oracle.NewErpAdjReader(oracleClient.ReadOnly())
		reader, err := oracle.NewErpDemandReader(oracleClient.ReadOnly(), cfg.ERP.DemandSource, cfg.App.Env)
		if err != nil {
			log.Error().Err(err).Msg("ERP demand reader disabled; load_demand jobs will fail")
		} else {
			load = erpapp.NewLoadDemandStep(runner, reader, oracle.NewErpAdjReader(oracleClient.ReadOnly()))
		}
	}
	coverage := erpapp.NewCoverageStep(runner, postgres.NewErpCoverageSourceRepository(db), cfg.ERP.DemandMaxAge)
	backfill := erpapp.NewBackfillAttributesHandler(legacyReader, postgres.NewErpAttrBackfillRepository(db),
		auditapp.NewEmitter(postgres.NewCostAuditLogRepository(db)))
	derive := erpapp.NewDeriveStep(runner, postgres.NewErpRuleSetLoader(db), cfg.ERP.DemandMaxAge)
	validate := erpapp.NewValidateStep(runner, prober, cfg.ERP.DemandMaxAge)
	// W1 push (plan-06 P5-T3): the writer comes from the fail-closed factory
	// (disabled by default; fake refused in production; oracle maps to
	// disabled until P8-T2). A factory error leaves the disabled writer, so
	// push jobs fail G2 with no Oracle call.
	writer, writerMode, werr := oracle.NewErpWriter(cfg.ERP, cfg.OracleIF, cfg.App.Env, log.Logger)
	if werr != nil {
		log.Warn().Err(werr).Str("erp_writer_mode", string(writerMode)).Msg("ERP writer not available; push jobs will fail closed")
	}
	gate := erpapp.WriterGate{Writer: writer, Mode: writerMode}
	push := erpapp.NewPushStep(runner, gate,
		erpmetrics.NewInstrumentedCallLog(postgres.NewErpOracleCallRepository(db)), auditapp.NewEmitter(postgres.NewCostAuditLogRepository(db)),
		cfg.ERP.PushEnabled, cfg.ERP.CallTimeout)
	// Backtest (plan-06 P5-T10b): SHADOW only, no writer. The legacy reader
	// stays a true nil interface without Oracle, so the handler fails closed.
	var btReader erpdomain.LegacyAdjRateReader
	if oracleClient != nil {
		btReader = oracle.NewLegacyAdjRateReader(oracleClient.ReadOnly())
	}
	backtest := erpapp.NewBacktestHandler(erpapp.NewCreateBatchHandler(postgres.NewErpIntBatchRepository(db), nil, nil),
		load, coverage, derive, validate, postgres.NewErpStdCostRepository(db), btReader)
	exec := erpapp.NewJobExecutor(jobRepo, runner, load, coverage).WithDerive(derive).WithValidate(validate).
		WithAttrBackfill(backfill).WithPush(push).
		WithBacktest(backtest, postgres.NewErpBacktestReportRepository(db))
	wireErpAdjSteps(exec, cfg, db, runner, gate, oracleClient, prober)
	stop := wireErpScheduler(ctx, exec, cfg, db, jobRepo, prober, publisher)
	return exec, stop
}

// runErpIntegrationJob dispatches an erp_integration message to the executor.
func runErpIntegrationJob(ctx context.Context, h *erpapp.JobExecutor, msg rabbitmq.JobMessage) error {
	jobID, err := uuid.Parse(msg.JobID)
	if err != nil {
		log.Error().Err(err).Str("job_id", msg.JobID).Msg("Invalid erp_integration job ID")
		return err
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
