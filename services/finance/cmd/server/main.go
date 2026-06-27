// Package main is the entry point for the finance service.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/robfig/cron/v3"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/pkg/costcalc/metrics"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/auditadapter"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/bi/bietl"
	chartdataapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/chartdata"
	dashboardapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/dashboard"
	datasourceapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/datasource"
	groupapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/group"
	jobapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/job"
	uploadapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/upload"
	auditapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costauditlog"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costbulkimport"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	fillapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costfillassignment"
	costnotifapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costnotification"
	cappapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductapplicableparam"
	cpmapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	cppapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductparameter"
	cprapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/oraclesync"
	apprmcost "github.com/mutugading/goapps-backend/services/finance/internal/application/rmcost"
	grpcdelivery "github.com/mutugading/goapps-backend/services/finance/internal/delivery/grpc"
	httpdelivery "github.com/mutugading/goapps-backend/services/finance/internal/delivery/httpdelivery"
	notifDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costnotification"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	fillnotifierinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/fillnotifier"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamclient"
	iamnotifier "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamnotifier"
	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
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
func run() error { //nolint:gocognit,gocyclo // linear service wiring / DI setup
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

	// Background DB-pool gauge scraper for cost-calc observability.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				metrics.DBPoolInUse.WithLabelValues("finance").Set(float64(db.Stats().InUse))
			}
		}
	}()

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
	boxBobbinCostRepo := postgres.NewBoxBobbinCostRepository(db)
	mbHeadRepo := postgres.NewMBHeadRepository(db)
	mbSpinRepo := postgres.NewMBSpinRepository(db)
	machineRepo := postgres.NewMachineRepository(db)
	interminglingRepo := postgres.NewInterminglingRepository(db)
	productGradeRepo := postgres.NewProductGradeRepository(db)
	lookupMasterRepo := postgres.NewLookupMasterRepository(db)
	// NOTE: legacy productRepo / prdRequestRepo wired to dropped tables — removed.
	// Canonical Phase B (cost_product_master, cost_product_order) wiring added in S2.8-S2.10.

	// Setup oracle sync handlers
	triggerHandler := oraclesync.NewTriggerHandler(jobRepo, oracleSyncPublisher)
	getJobHandler := oraclesync.NewGetJobHandler(jobRepo)
	listJobsHandler := oraclesync.NewListJobsHandler(jobRepo)
	cancelJobHandler := oraclesync.NewCancelJobHandler(jobRepo)
	listDataHandler := oraclesync.NewListDataHandler(syncDataRepo)
	listPeriodsHandler := oraclesync.NewListPeriodsHandler(syncDataRepo)

	// BI repositories + cache
	biDashboardRepo := postgres.NewBiDashboardRepository(db)
	biGroupRepo := postgres.NewBiDashboardGroupRepository(db)
	biFactRepo := postgres.NewBiFactMetricRepository(db)
	biDataSourceRepo := postgres.NewBiDataSourceRepository(db)
	biJobRepo := postgres.NewBiJobRepository(db)
	biUploadRepo := postgres.NewBiUploadRepository(db)
	biAuditRepo := postgres.NewBiAuditRepository(db)
	biChartCache := redisinfra.NewChartCache(redisClient)

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

	boxBobbinCostHandler, err := grpcdelivery.NewBoxBobbinCostHandler(boxBobbinCostRepo)
	if err != nil {
		return err
	}

	mbHeadHandler, err := grpcdelivery.NewMBHeadHandler(mbHeadRepo)
	if err != nil {
		return err
	}

	mbSpinHandler, err := grpcdelivery.NewMBSpinHandler(mbSpinRepo)
	if err != nil {
		return err
	}

	machineHandler, err := grpcdelivery.NewMachineHandler(machineRepo)
	if err != nil {
		return err
	}

	interminglingHandler, err := grpcdelivery.NewInterminglingHandler(interminglingRepo)
	if err != nil {
		return err
	}

	productGradeHandler, err := grpcdelivery.NewProductGradeHandler(productGradeRepo)
	if err != nil {
		return err
	}

	lookupMasterHandler, err := grpcdelivery.NewLookupMasterHandler(lookupMasterRepo)
	if err != nil {
		return fmt.Errorf("new lookup master handler: %w", err)
	}

	yarnLookupFillHandler, err := grpcdelivery.NewYarnLookupFillHandler(
		machineRepo, interminglingRepo, productGradeRepo, mbHeadRepo, mbSpinRepo, boxBobbinCostRepo, parameterRepo,
	)
	if err != nil {
		return fmt.Errorf("new yarn lookup fill handler: %w", err)
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
	costImportJobRepo := postgres.NewCostImportJobRepository(db)
	requestHistoryRepo := postgres.NewRequestHistoryRepository(db)

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

	// Wire async import support (storage + job repo + publisher) into CPM handler.
	costProductMasterHandler.WithImportSupport(costImportJobRepo, storageSvc, rmqAdapter)
	costProductMasterHandler.WithAuditSupport(costAuditLogRepo)

	// Build CostDataImportHandler (CAPP/CPP async import + export/template for CAPP/CPP/CPM).
	cappExportH := cappapp.NewExportHandler(costProductParameterRepo)
	cappTemplateH := cappapp.NewTemplateHandler()
	cppExportH := cppapp.NewExportHandler(costProductParameterRepo)
	cppTemplateH := cppapp.NewTemplateHandler()
	cpmExportH := cpmapp.NewExportHandler(costProductMasterRepo)
	cpmTemplateH := cpmapp.NewTemplateHandler()
	bulkValidateH := costbulkimport.NewValidateHandler(costProductParameterRepo, costProductTypeRepo)
	bulkTemplateH := costbulkimport.NewTemplateHandler()
	costDataImportHandler := grpcdelivery.NewCostDataImportHandler(
		costImportJobRepo, storageSvc,
		cappExportH, cappTemplateH,
		cppExportH, cppTemplateH,
		cpmExportH, cpmTemplateH,
		rmqAdapter,
		bulkValidateH,
		bulkTemplateH,
	)

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
	// Shared notification emitter used by both gRPC handlers and cron jobs.
	costNotifEmitter := costnotifapp.NewEmitter(costNotificationRepo)

	// IAM notification client — used by CPRNotifier and FillNotifier for rule-based
	// fan-out to multiple recipients. Falls back to nop client on dial failure so
	// the server still starts (notifications are best-effort).
	iamNotifClient, iamNotifErr := iamclient.NewClient(
		cfg.IAMClient.Host,
		cfg.IAMClient.Port,
		cfg.IAMClient.InternalServiceToken,
	)
	if iamNotifErr != nil {
		log.Warn().Err(iamNotifErr).Msg("IAM notification client dial failed; using nop (notifications disabled)")
		iamNotifClient = iamclient.NewNopClient()
	} else {
		defer func() {
			if closeErr := iamNotifClient.Close(); closeErr != nil {
				log.Warn().Err(closeErr).Msg("IAM notification client close error")
			}
		}()
	}

	cprIAMNotifier := iamnotifier.NewCPRNotifier(iamNotifClient)
	fillIAMNotifier := iamnotifier.NewFillNotifier(iamNotifClient)

	costProductParameterApp := cppapp.New(costProductParameterRepo)
	costProductParameterHandler := grpcdelivery.NewCostProductParameterHandler(costProductParameterApp).
		WithParamRepo(parameterRepo).
		WithAuditSupport(costAuditLogRepo)

	// Fill-assignment repositories + handlers.
	fillConfigRepo := postgres.NewCostFillConfigRepository(db)
	fillTaskRepo := postgres.NewCostFillTaskRepository(db)
	// Wire fill-task approval check into the route lock handler.
	// Only tasks with an approver configured block locking; no-approver levels are ignored.
	costRouteHandler.WithFillApprovalChecker(fillTaskRepo)

	upsertGlobalHandler := fillapp.NewUpsertGlobalConfigHandler(fillConfigRepo)
	upsertOverrideHandler := fillapp.NewUpsertOverrideHandler(fillConfigRepo)
	deleteGlobalHandler := fillapp.NewDeleteGlobalConfigHandler(fillConfigRepo)
	listGlobalHandler := fillapp.NewListGlobalConfigHandler(fillConfigRepo)

	createAllTasksHandler := fillapp.NewCreateAllTasksHandler(fillConfigRepo, fillTaskRepo)
	createAllTasksHandler.WithNotifier(fillIAMNotifier)
	costProductRequestHandler.WithFillCreator(createAllTasksHandler)
	costProductRequestHandler.WithFillChecker(fillTaskRepo)
	costProductRequestHandler.WithRouteLockChecker(costRouteRepo)
	costProductRequestHandler.WithParamCounter(costProductParameterRepo)

	// Wire in-app notification emitter to the CPR TransitionHandler so that
	// status-change events (Submit, StartReview, Reject, etc.) produce persisted
	// notifications visible in the frontend bell icon.
	costProductRequestHandler.WithNotifier(&cprNotifEmitterAdapter{emitter: costNotifEmitter})

	// Wire IAM-backed CPR notifier for rule-based multi-recipient fan-out.
	costProductRequestHandler.WithCPRNotifier(cprIAMNotifier)

	// Wire CPR notifier into the comment handler so that CPR_COMMENT_ADDED
	// notifications are emitted whenever a new comment is posted.
	costRequestCommentHandler.WithCPRNotifier(costProductRequestRepo, cprIAMNotifier)

	// Wire approval trace repository so every state transition is recorded and
	// the GetCostProductRequestHistory RPC is enabled.
	costProductRequestHandler.WithHistoryRepo(requestHistoryRepo)

	// Wire param edit log repo (audit trail for param value overrides).
	paramEditLogRepo := postgres.NewCostParamEditLogRepository(db.DB)
	overrideParamHandler := cppapp.NewOverrideParamValuesHandler(costProductParameterRepo, costRouteRepo, paramEditLogRepo)
	overrideParamHandler.WithTaskResetter(fillTaskRepo)
	costProductParameterHandler.WithOverrideHandler(overrideParamHandler)
	costProductParameterHandler.WithEditLogRepo(paramEditLogRepo)

	// Wire param summary handler for GetParamSummary RPC.
	paramSummaryRepo := postgres.NewParamSummaryRepository(db)
	paramSummaryHandler := cprapp.NewGetParamSummaryHandler(paramSummaryRepo).WithEditLogLoader(paramEditLogRepo)
	costProductRequestHandler.WithParamSummary(paramSummaryHandler)

	// Build the completion gate: L100-L102 chain creation + CPR state machine trigger.
	cprCompleter := &cprCompleterAdapter{handler: costProductRequestHandler}
	completionNotifier := &completionNotifierAdapter{emitter: costNotifEmitter}
	completionGate := fillapp.NewCompletionGateHandler(fillTaskRepo, fillConfigRepo, cprCompleter, completionNotifier)
	completionGate.WithFillNotifier(fillIAMNotifier)

	costFillConfigHandler := grpcdelivery.NewCostFillConfigHandler(
		upsertGlobalHandler, upsertOverrideHandler, deleteGlobalHandler, listGlobalHandler,
	)
	costFillTaskHandler := grpcdelivery.NewCostFillTaskHandler(fillTaskRepo, completionGate)
	costFillTaskHandler.WithSubmitFillNotifier(fillIAMNotifier, &cprRequestNoAdapter{repo: costProductRequestRepo})

	// SLA + reminder cron jobs for fill-assignment notifications.
	// reminderGapHours=4: at most one reminder per task per 4 hours.
	const reminderGapHours = 4
	fillSLANotifier := fillnotifierinfra.New(fillTaskRepo, costNotifEmitter)
	slaJob := fillapp.NewSLANotifierJob(fillTaskRepo, fillSLANotifier, reminderGapHours)
	slaJob.WithFillNotifier(fillIAMNotifier)
	reminderJob := fillnotifierinfra.NewReminderJob(fillTaskRepo, costNotifEmitter, reminderGapHours)
	reminderJob.WithFillNotifier(fillIAMNotifier)
	fillCron := cron.New()
	if _, addErr := fillCron.AddFunc("0 * * * *", slaJob.Run); addErr != nil {
		return fmt.Errorf("register sla notifier cron: %w", addErr)
	}
	if _, addErr := fillCron.AddFunc("30 * * * *", reminderJob.Run); addErr != nil {
		return fmt.Errorf("register reminder cron: %w", addErr)
	}
	fillCron.Start()
	defer fillCron.Stop()

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
		costcalc.NewListCostResultsHandler(calcSvc),
		costcalc.NewVerifyCostHandler(calcSvc),
		costcalc.NewApproveCostHandler(calcSvc),
	)

	// BI (Executive Dashboard) gRPC handlers.
	biDashboardHandler, err := grpcdelivery.NewBIDashboardHandler(
		dashboardapp.NewCreateHandler(biDashboardRepo),
		dashboardapp.NewGetHandler(biDashboardRepo),
		dashboardapp.NewListHandler(biDashboardRepo),
		dashboardapp.NewUpdateHandler(biDashboardRepo, biChartCache),
		dashboardapp.NewDeleteHandler(biDashboardRepo, biChartCache),
		dashboardapp.NewDuplicateHandler(biDashboardRepo),
		dashboardapp.NewSetRolesHandler(biDashboardRepo, biChartCache),
		dashboardapp.NewListAccessibleHandler(biDashboardRepo),
		dashboardapp.NewListFeaturedHandler(biDashboardRepo),
		groupapp.NewCreateHandler(biGroupRepo),
		groupapp.NewListHandler(biGroupRepo),
		groupapp.NewUpdateHandler(biGroupRepo),
		groupapp.NewDeleteHandler(biGroupRepo),
		biAuditRepo,
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

	// Oracle BI ETL runner (optional — graceful degradation when Oracle is unavailable).
	var biETLRunner jobapp.BIETLRunner
	oracleClient, oracleErr := oracleinfra.NewClient(cfg.Oracle, log.Logger)
	if oracleErr != nil {
		log.Warn().Err(oracleErr).Msg("Oracle unavailable; BI ETL jobs (etl_mis/etl_delivery_margin/etl_sales) will fail gracefully")
	} else {
		defer func() {
			if cErr := oracleClient.Close(); cErr != nil {
				log.Warn().Err(cErr).Msg("Failed to close Oracle connection")
			}
		}()
		biMVRepo := oracleinfra.NewBIMVRepository(oracleClient)
		biETLRunner = bietl.NewMVLoader(biMVRepo, biFactRepo)
	}

	biMVRefresher := &biMVRefresherAdapter{db: db}
	biJobTriggerHandler := jobapp.NewTriggerHandler(biJobRepo, biMVRefresher, biETLRunner, biChartCache)
	biJobHandler, err := grpcdelivery.NewBIJobHandler(
		jobapp.NewListHandler(biJobRepo),
		jobapp.NewListLogsHandler(biJobRepo),
		biJobTriggerHandler,
		jobapp.NewCreateHandler(biJobRepo),
		jobapp.NewUpdateHandler(biJobRepo),
		jobapp.NewDeleteHandler(biJobRepo),
	)
	if err != nil {
		return fmt.Errorf("create BI job handler: %w", err)
	}
	// BI job scheduler — fires ETL/MV_REFRESH jobs automatically on their cron schedule.
	// Syncs active jobs from DB every 5 minutes to pick up admin changes without restart.
	biJobScheduler := jobapp.NewBiJobScheduler(biJobRepo, biJobTriggerHandler, log.Logger, 5*time.Minute)
	go biJobScheduler.Start(ctx)

	excelUploadSourceLookup := func(ctx context.Context) (uuid.UUID, error) {
		ds, err := biDataSourceRepo.GetByCode(ctx, "EXCEL_UPLOAD")
		if err != nil {
			return uuid.Nil, err
		}
		return ds.ID, nil
	}
	biUploadHandler, err := grpcdelivery.NewBIUploadHandler(
		uploadapp.NewTemplateHandler(),
		uploadapp.NewParseHandler(biUploadRepo, excelUploadSourceLookup),
		uploadapp.NewCommitHandler(biUploadRepo),
		uploadapp.NewCancelHandler(biUploadRepo),
		uploadapp.NewListHandler(biUploadRepo),
	)
	if err != nil {
		return err
	}

	// Setup and start servers
	return startServers(ctx, cfg,
		uomHandler, rmCategoryHandler, parameterHandler, formulaHandler, uomCategoryHandler,
		boxBobbinCostHandler,
		mbHeadHandler, mbSpinHandler,
		machineHandler, interminglingHandler, productGradeHandler, lookupMasterHandler, yarnLookupFillHandler,
		oracleSyncHandler, rmGroupHandler, rmCostHandler,
		costProductTypeHandler, costRmTypeHandler, costErpHandler, costProductMasterHandler, costRouteHandler,
		costRequestTypeHandler, costPaperTubeTypeHandler, costProductRequestHandler,
		costRequestCommentHandler, costAttachmentHandler,
		costRoutingRuleHandler, costAuditLogHandler, costNotificationHandler,
		costProductParameterHandler,
		costDataImportHandler,
		costCalcHandler,
		costFillConfigHandler, costFillTaskHandler,
		biDashboardHandler, biChartDataHandler, biDataSourceHandler, biJobHandler, biUploadHandler,
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
	boxBobbinCostHandler *grpcdelivery.BoxBobbinCostHandler,
	mbHeadHandler *grpcdelivery.MBHeadHandler,
	mbSpinHandler *grpcdelivery.MBSpinHandler,
	machineHandler *grpcdelivery.MachineHandler,
	interminglingHandler *grpcdelivery.InterminglingHandler,
	productGradeHandler *grpcdelivery.ProductGradeHandler,
	lookupMasterHandler *grpcdelivery.LookupMasterHandler,
	yarnLookupFillHandler *grpcdelivery.YarnLookupFillHandler,
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
	costDataImportHandler *grpcdelivery.CostDataImportHandler,
	costCalcHandler *grpcdelivery.CostCalcHandler,
	costFillConfigHandler *grpcdelivery.CostFillConfigHandler,
	costFillTaskHandler *grpcdelivery.CostFillTaskHandler,
	biDashboardHandler *grpcdelivery.BIDashboardHandler,
	biChartDataHandler *grpcdelivery.BIChartDataHandler,
	biDataSourceHandler *grpcdelivery.BIDataSourceHandler,
	biJobHandler *grpcdelivery.BIJobHandler,
	biUploadHandler *grpcdelivery.BIUploadHandler,
	tokenBlacklist *redisinfra.TokenBlacklist,
) error {
	// Setup gRPC server with JWT auth and token blacklist
	grpcServer, err := grpcdelivery.NewServer(&cfg.Server, nil, &cfg.JWT, tokenBlacklist, tokenBlacklist)
	if err != nil {
		return err
	}

	// Register services
	financev1.RegisterUOMServiceServer(grpcServer.GRPCServer(), uomHandler)
	financev1.RegisterRMCategoryServiceServer(grpcServer.GRPCServer(), rmCategoryHandler)
	financev1.RegisterParameterServiceServer(grpcServer.GRPCServer(), parameterHandler)
	financev1.RegisterFormulaServiceServer(grpcServer.GRPCServer(), formulaHandler)
	financev1.RegisterUOMCategoryServiceServer(grpcServer.GRPCServer(), uomCategoryHandler)
	financev1.RegisterBoxBobbinCostServiceServer(grpcServer.GRPCServer(), boxBobbinCostHandler)
	financev1.RegisterMBHeadServiceServer(grpcServer.GRPCServer(), mbHeadHandler)
	financev1.RegisterMBSpinServiceServer(grpcServer.GRPCServer(), mbSpinHandler)
	// Yarn master services.
	financev1.RegisterMachineServiceServer(grpcServer.GRPCServer(), machineHandler)
	financev1.RegisterInterminglingServiceServer(grpcServer.GRPCServer(), interminglingHandler)
	financev1.RegisterProductGradeServiceServer(grpcServer.GRPCServer(), productGradeHandler)
	financev1.RegisterLookupMasterServiceServer(grpcServer.GRPCServer(), lookupMasterHandler)
	financev1.RegisterYarnLookupFillServiceServer(grpcServer.GRPCServer(), yarnLookupFillHandler)
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
	// Costing data import/export service.
	financev1.RegisterCostDataImportServiceServer(grpcServer.GRPCServer(), costDataImportHandler)
	// S8a foundation: CostCalcService stub.
	financev1.RegisterCostCalcServiceServer(grpcServer.GRPCServer(), costCalcHandler)

	// Fill-assignment services.
	financev1.RegisterCostLevelAssignmentConfigServiceServer(grpcServer.GRPCServer(), costFillConfigHandler)
	financev1.RegisterCostFillTaskServiceServer(grpcServer.GRPCServer(), costFillTaskHandler)

	// BI services
	financev1.RegisterDashboardServiceServer(grpcServer.GRPCServer(), biDashboardHandler)
	financev1.RegisterChartDataServiceServer(grpcServer.GRPCServer(), biChartDataHandler)
	financev1.RegisterDataSourceServiceServer(grpcServer.GRPCServer(), biDataSourceHandler)
	financev1.RegisterBiJobServiceServer(grpcServer.GRPCServer(), biJobHandler)
	financev1.RegisterBiUploadServiceServer(grpcServer.GRPCServer(), biUploadHandler)

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

// biMVRefresherAdapter implements jobapp.MVRefresher using a raw DB connection.
// It calls the bi_refresh_dashboard_mvs() PostgreSQL function which refreshes
// all BI materialized views (mv_bi_metric_g1, mv_bi_metric_g2) concurrently.
type biMVRefresherAdapter struct{ db *postgres.DB }

func (a *biMVRefresherAdapter) RefreshMVs(ctx context.Context) error {
	if _, err := a.db.ExecContext(ctx, "SELECT bi_refresh_dashboard_mvs()"); err != nil {
		return fmt.Errorf("bi_refresh_dashboard_mvs: %w", err)
	}
	return nil
}

// =============================================================================
// Fill-assignment completion adapters
// =============================================================================

// cprCompleterAdapter wraps CostProductRequestHandler to implement
// fillapp.CPRCompleter, allowing the CompletionGateHandler (application layer)
// to trigger the PARAMETER_COMPLETE state transition without importing the
// delivery package (the adapter lives in main.go, which bridges both).
type cprCompleterAdapter struct {
	handler *grpcdelivery.CostProductRequestHandler
}

func (a *cprCompleterAdapter) MarkParameterComplete(ctx context.Context, requestID int64, actor string) (string, string, error) {
	return a.handler.MarkParameterCompleteForGate(ctx, requestID, actor)
}

// completionNotifierAdapter wraps costnotifapp.Emitter to implement
// fillapp.CompletionNotifier for the L100-102 completion chain notifications.
type completionNotifierAdapter struct {
	emitter *costnotifapp.Emitter
}

func (a *completionNotifierAdapter) NotifyFiller(ctx context.Context, taskID int64, recipientUserID, requestNo string) error {
	payload := fmt.Sprintf(`{"taskId":%d,"requestNo":%q}`, taskID, requestNo)
	_, err := a.emitter.Emit(ctx, notifDomain.NewInput{
		RecipientUserID: recipientUserID,
		TriggerType:     notifDomain.TriggerPendingFill,
		Payload:         payload,
	})
	return err
}

func (a *completionNotifierAdapter) NotifyComplete(ctx context.Context, requestID int64, requesterUserID, requestNo string) error {
	payload := fmt.Sprintf(`{"status":"PARAMETER_COMPLETE","requestNo":%q}`, requestNo)
	_, err := a.emitter.Emit(ctx, notifDomain.NewInput{
		RecipientUserID: requesterUserID,
		TriggerType:     notifDomain.TriggerStatusChange,
		RequestID:       requestID,
		Payload:         payload,
	})
	return err
}

// cprNotifEmitterAdapter wraps costnotifapp.Emitter to implement
// costproductrequest.NotificationEmitter, enabling the TransitionHandler to
// send in-app notifications without importing the costnotification package.
type cprNotifEmitterAdapter struct {
	emitter *costnotifapp.Emitter
}

func (a *cprNotifEmitterAdapter) Emit(ctx context.Context, in cprapp.NotificationInput) error {
	_, err := a.emitter.Emit(ctx, notifDomain.NewInput{
		RecipientUserID: in.RecipientUserID,
		TriggerType:     in.TriggerType,
		RequestID:       in.RequestID,
		Payload:         in.Payload,
	})
	return err
}

// cprRequestNoAdapter implements fillapp.RequestNoProvider by delegating to the
// CostProductRequestRepository. Used by SubmitFillHandler to resolve request_no
// for the NotifyApprovalPending notification.
type cprRequestNoAdapter struct {
	repo *postgres.CostProductRequestRepository
}

func (a *cprRequestNoAdapter) GetRequestNo(ctx context.Context, requestID int64) (string, error) {
	req, err := a.repo.GetByID(ctx, requestID)
	if err != nil {
		return "", fmt.Errorf("cprRequestNoAdapter.GetRequestNo: %w", err)
	}
	return req.RequestNo(), nil
}
