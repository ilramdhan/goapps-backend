// Package main is the entry point for the PPC (Production Planning & Control) service.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	bfsapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/balanceforsale"
	capacityapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/capacity"
	changeoverapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/changeover"
	commonlotapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/commonlot"
	customerapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/customer"
	dpapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/dailyperf"
	dashapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/dashboard"
	demandapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/demand"
	downtimereasonapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/downtimereason"
	etlapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/etl"
	lookupapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/lookup"
	lotapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/lot"
	machineapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/machine"
	machinegroupapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/machinegroup"
	"github.com/mutugading/goapps-backend/services/ppc/internal/application/machinesync"
	planitemapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/planitem"
	productconfigapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/productconfig"
	pmpapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/productmachineparameter"
	shiftapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/shift"
	thresholdapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/threshold"
	wastecategoryapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/wastecategory"
	workorderapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/workorder"
	grpcdelivery "github.com/mutugading/goapps-backend/services/ppc/internal/delivery/grpc"
	httpdelivery "github.com/mutugading/goapps-backend/services/ppc/internal/delivery/httpdelivery"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/financeclient"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/metrics"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/tracing"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/worker"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("Service failed")
	}
}

// run contains the main application logic, separated for cleaner error handling.
func run() error {
	setupLogger()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log.Info().
		Str("service", cfg.App.Name).
		Str("version", cfg.App.Version).
		Str("environment", cfg.App.Env).
		Msg("Starting PPC service")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup tracing (optional).
	cleanupTracing := setupTracing(ctx, cfg)
	defer cleanupTracing()

	// Setup database.
	db, err := setupDatabase(cfg)
	if err != nil {
		return err
	}
	defer closeDatabase(db)

	// Background DB-pool gauge scraper for observability.
	go scrapeDBPool(ctx, db)

	// Finance gRPC clients (master reads + machine sync). Empty host → degraded.
	financeCfg := &cfg.FinanceClient
	callTimeout := cfg.Server.GRPCTimeout
	lookupClient, err := financeclient.New(financeCfg.Host, financeCfg.Port, financeCfg.InternalServiceToken, callTimeout)
	if err != nil {
		return err
	}
	defer closeQuietly("finance lookup client", lookupClient.Close)

	machineClient, err := financeclient.NewMachineClient(financeCfg.Host, financeCfg.Port, financeCfg.InternalServiceToken, callTimeout)
	if err != nil {
		return err
	}
	defer closeQuietly("finance machine client", machineClient.Close)

	// Oracle client for TXTMACH machine source. Unreachable/unconfigured → nil,
	// and the sync degrades to finance-only.
	oracleClient, err := oracle.New(cfg.Oracle)
	if err != nil {
		log.Warn().Err(err).Msg("Oracle unavailable, machine sync will run finance-only")
		oracleClient = nil
	}
	defer closeQuietly("oracle client", oracleClient.Close)

	// Machine-sync usecase + worker.
	machineRepo := postgres.NewMachineRepository(db)
	syncUsecase := machinesync.NewUsecase(machineRepo, machineClient, oracleSource(oracleClient))
	syncWorker := worker.NewMachineSyncWorker(syncUsecase, cfg.MachineSync.Interval)
	go syncWorker.Start(ctx)

	// Parameter-resolution chain (v1.2): WO-ref → product_machine → product(finance)
	// → mst_parameter default. Finance-backed layers degrade gracefully.
	woRepo := postgres.NewWorkOrderRepository(db)
	resolver := workorderdomain.NewResolver(
		financeclient.NewParamDefSource(lookupClient),
		postgres.NewProductMachineValueSource(db),
		financeclient.NewProductValueSource(lookupClient),
		postgres.NewWORefValueSource(db),
	)

	// Layer-3 WO service (shared by the gRPC handler + auto-approve worker).
	// Notifier + snapshot builder are nil in Phase-1 (wired later).
	woService := workorderapp.NewService(woRepo, workorderapp.Deps{
		Machines:     postgres.NewMachineAreaLookup(machineRepo),
		MachineNames: postgres.NewMachineNoLookup(machineRepo),
		Lots:         postgres.NewLotExistsLookup(postgres.NewLotRepository(db)),
		PlanItems:    postgres.NewPlanItemProductLookup(db),
		Resolver:     resolver,
		RouteRms:     financeclient.NewRouteRmSource(lookupClient),
		Merge:        postgres.NewMergeCandidateLookup(db),
		LotSpecs:     financeclient.NewLotSpecSource(lookupClient),
		LotProv:      postgres.NewLotProvisioner(db),
	})

	// Dual-approval auto-approve worker (1-min ticker, AUTO_APPROVE_HOURS window).
	autoApproveWorker := worker.NewAutoApproveWorker(woService, cfg.Approval.AutoApproveHours, time.Minute)
	go autoApproveWorker.Start(ctx)

	// Customer master: read-only Oracle OM_CUSTOMER sync + CRUD. A nil Oracle
	// client makes the sync a no-op rather than an error.
	customerRepo := postgres.NewCustomerRepository(db)
	customerService := customerapp.NewService(customerRepo).
		WithSync(customerapp.NewSyncUsecase(customerRepo, customerapp.NewOracleSource(oracleCustomerSource(oracleClient))))

	// Lot master: read-only ASPAK.MMSMERGE import + CRUD. Same degradation
	// contract as the customer sync — a nil Oracle client makes it a no-op.
	lotRepo := postgres.NewLotRepository(db)
	lotService := lotapp.NewService(lotRepo).
		WithSync(lotapp.NewSyncUsecase(lotRepo, oracleLotSource(oracleClient)))

	// Layer-1 demand service. Staging rows are resolved to finance products over
	// gRPC (separate databases), both after each ETL sync and lazily on read.
	// Customer lives in ppc_db, so it resolves through a direct code lookup.
	demandService := demandapp.NewService(postgres.NewDemandRepository(db), lookupClient, nil).
		WithProductResolution(financeclient.NewStagingResolver(lookupClient), lookupClient).
		WithCustomerResolution(customerService)

	// ETL (two-axis v1.2): TXT production + SO staging workers + suggest chain.
	// Oracle-unreachable → oracleClient is nil and the sources degrade to no-op.
	etlRepo := postgres.NewETLRepository(db)
	suggestService := etlapp.NewSuggestService(etlRepo)
	txtETL := etlapp.NewTxtProductionETL(oracleTxtSource(oracleClient), etlRepo, cfg.ETL.WatermarkBufferMinutes)
	spgETL := etlapp.NewSpgProductionETL(oracleSpgSource(oracleClient), etlRepo, cfg.ETL.WatermarkBufferMinutes)
	soETL := etlapp.NewSoStagingETL(oracleSoSource(oracleClient), etlRepo).
		WithProductResolver(demandService)
	// Packing grade-actual ETL (Phase 3): PPC_GRADE_ACTUAL → wo_grade_actual,
	// which makes the suggest-chain P1 (PACKING_DONE) branch live.
	gradeETL := etlapp.NewGradeActualETL(oracleGradeSource(oracleClient), etlRepo, cfg.ETL.WatermarkBufferMinutes)
	go worker.NewTxtProductionETLWorker(txtETL, time.Duration(cfg.ETL.IntervalMinutes)*time.Minute).Start(ctx)
	go worker.NewSpgProductionETLWorker(spgETL, time.Duration(cfg.ETL.IntervalMinutes)*time.Minute).Start(ctx)
	go worker.NewSoStagingETLWorker(soETL, time.Duration(cfg.ETL.SOIntervalMinutes)*time.Minute).Start(ctx)
	go worker.NewGradeActualETLWorker(gradeETL, time.Duration(cfg.ETL.IntervalMinutes)*time.Minute).Start(ctx)

	// Daily Performance (v1.2): shift-entry, efficiency engine (derived-only from
	// well-known pinned params), snapshot roll-up worker, shift-log notes.
	dailyPerfService := buildDailyPerfService(db, resolver)
	go worker.NewEfficiencySnapshotWorker(dailyPerfService, time.Duration(cfg.ETL.IntervalMinutes)*time.Minute).Start(ctx)

	// Changeover (Phase-2): component-based detection + estimate/actual capture.
	changeoverService := changeoverapp.NewService(
		postgres.NewChangeoverRepository(db),
		postgres.NewChangeoverWOSpecSource(db),
		nil, // PRD default component table (master override is future work)
	)

	// Balance-for-Sale (Phase-2): AX balance per commodity-watch product.
	// current_stock_AX is stubbed to 0 (no Orion inventory ETL in scope).
	// Notifier is nil in this phase (IAM-backed notifier wired later); the BFS
	// commodity-watch notification degrades to a no-op via notification.Notify.
	bfsService := bfsapp.NewService(postgres.NewBalanceForSaleRepository(db), lookupClient, nil)

	// Read dashboards (Phase-1 plan-06): daily performance (KPI + MC-EFF heatmap)
	// and morning review, aggregated from efficiency snapshots + planning data.
	dashboardReadService := dashapp.NewService(postgres.NewDashboardRepository(db))

	// Setup gRPC handlers. Master + planning + ETL/suggest + daily-performance +
	// changeover + balance-for-sale RPCs are implemented; remaining dashboard RPCs
	// fall through to Unimplemented.
	ppcHandler := buildPPCHandler(ppcDeps{
		db:             db,
		demandService:  demandService,
		machineRepo:    machineRepo,
		validator:      lookupClient,
		syncUsecase:    syncUsecase,
		woService:      woService,
		suggestService: suggestService,
		dailyPerf:      dailyPerfService,
		changeover:     changeoverService,
		dashboard:      bfsService,
		dashboardRead:  dashboardReadService,
		etlRepo:        etlRepo,
		customer:       customerService,
		lot:            lotService,
	})

	return startServers(ctx, cfg, ppcHandler)
}

// oracleSource returns the Oracle machine source, or nil when Oracle is
// unavailable so the sync usecase degrades to finance-only. Returning a typed
// nil directly would create a non-nil interface, so map it explicitly.
func oracleSource(c *oracle.Client) machinesync.OracleMachineSource {
	if c == nil {
		return nil
	}
	return c
}

// oracleCustomerSource maps the Oracle client to the customer read port, or nil
// when Oracle is unavailable so the customer sync degrades to a no-op.
func oracleCustomerSource(c *oracle.Client) customerapp.OracleReader {
	if c == nil {
		return nil
	}
	return c
}

// oracleLotSource maps the Oracle client to the lot read port, or nil when
// Oracle is unavailable so the lot sync degrades to a no-op. Returning the
// typed nil directly would yield a non-nil interface, so map it explicitly.
func oracleLotSource(c *oracle.Client) lotapp.OracleLotSource {
	if c == nil {
		return nil
	}
	return c
}

// oracleTxtSource maps the Oracle client to the TXT-production ETL source, or nil
// when Oracle is unavailable so the ETL degrades to a no-op.
func oracleTxtSource(c *oracle.Client) etlapp.TxtProductionSource {
	if c == nil {
		return nil
	}
	return c
}

// oracleSpgSource maps the Oracle client to the SPG-production ETL source, or
// nil when Oracle is unavailable so the ETL degrades to a no-op.
func oracleSpgSource(c *oracle.Client) etlapp.SpgProductionSource {
	if c == nil {
		return nil
	}
	return c
}

// oracleSoSource maps the Oracle client to the SO-pending ETL source, or nil when
// Oracle is unavailable so the ETL degrades to a no-op.
func oracleSoSource(c *oracle.Client) etlapp.SoPendingSource {
	if c == nil {
		return nil
	}
	return c
}

// oracleGradeSource maps the Oracle client to the packing grade-actual ETL source,
// or nil when Oracle is unavailable so the ETL degrades to a no-op.
func oracleGradeSource(c *oracle.Client) etlapp.GradeActualSource {
	if c == nil {
		return nil
	}
	return c
}

// buildDailyPerfService wires the Daily Performance application service from the
// daily-perf repository plus the shared parameter resolver (for well-known
// efficiency params). The base repository satisfies most ports directly; the
// named adapters disambiguate the Upsert/Replace method sets.
func buildDailyPerfService(db *postgres.DB, resolver *workorderdomain.Resolver) *dpapp.Service {
	repo := postgres.NewDailyPerfRepository(db)
	return dpapp.NewService(dpapp.Deps{
		ShiftLogs:        repo,
		AreaShiftLogs:    postgres.NewAreaShiftLogRepo(repo),
		Downtime:         postgres.NewDowntimeRepo(repo),
		Waste:            postgres.NewWasteRepo(repo),
		Notes:            postgres.NewNoteRepo(repo),
		Snapshots:        postgres.NewSnapshotRepo(repo),
		ProductionReader: repo,
		DowntimeReader:   repo,
		WasteReader:      repo,
		ShiftLogReader:   repo,
		WellKnown:        dpapp.NewWellKnownSource(resolver, repo),
		MachineNo:        repo,
	})
}

// ppcDeps groups the pre-built application services and repositories the
// composite PPC handler needs.
type ppcDeps struct {
	db             *postgres.DB
	machineRepo    *postgres.MachineRepository
	validator      *financeclient.Client
	syncUsecase    *machinesync.Usecase
	woService      *workorderapp.Service
	suggestService *etlapp.SuggestService
	dailyPerf      *dpapp.Service
	changeover     *changeoverapp.Service
	dashboard      *bfsapp.Service
	dashboardRead  *dashapp.Service
	etlRepo        *postgres.ETLRepository
	demandService  *demandapp.Service
	customer       *customerapp.Service
	lot            *lotapp.Service
}

// buildPPCHandler wires repositories and application services into the composite
// PPC gRPC handler.
func buildPPCHandler(d ppcDeps) *grpcdelivery.PPCHandler {
	db := d.db
	return grpcdelivery.NewPPCHandler(grpcdelivery.Deps{
		MachineGroup:            machinegroupapp.NewService(postgres.NewMachineGroupRepository(db)),
		Machine:                 machineapp.NewService(d.machineRepo),
		MachineSync:             d.syncUsecase,
		Customer:                d.customer,
		Lot:                     d.lot,
		ProductConfig:           productconfigapp.NewService(postgres.NewProductConfigRepository(db), d.validator),
		Capacity:                capacityapp.NewService(postgres.NewCapacityRepository(db), d.validator),
		ProductMachineParameter: pmpapp.NewService(postgres.NewProductMachineParameterRepository(db), d.validator),
		Threshold:               thresholdapp.NewService(postgres.NewThresholdRepository(db)),
		DowntimeReason:          downtimereasonapp.NewService(postgres.NewDowntimeReasonRepository(db)),
		WasteCategory:           wastecategoryapp.NewService(postgres.NewWasteCategoryRepository(db)),
		Demand:                  d.demandService,
		PlanItem: planitemapp.NewService(postgres.NewPlanItemRepository(db), d.validator, d.validator).
			WithCapacity(postgres.NewCapacityRepository(db)).
			WithRoutes(d.validator).
			WithMachineGroups(postgres.NewProductConfigRepository(db)).
			WithDemandLinks(d.demandService),
		WorkOrder:         d.woService,
		Suggest:           d.suggestService,
		DailyPerf:         d.dailyPerf,
		Changeover:        d.changeover,
		Dashboard:         d.dashboard,
		DashboardRead:     d.dashboardRead,
		SnapshotReader:    postgres.NewDailyPerfRepository(db),
		GradeActuals:      etlapp.NewGradeActualService(d.etlRepo),
		CommonLot:         commonlotapp.NewService(postgres.NewCommonLotRepository(db)),
		DailyPerfExporter: dpapp.NewExporter(postgres.NewDailyPerfRepository(db)),
		Lookup:            lookupapp.NewService(postgres.NewLookupRepository(db)),
		Shift:             shiftapp.NewService(postgres.NewShiftRepository(db)),
	})
}

// closeQuietly runs a close function and logs any error without failing.
func closeQuietly(name string, closeFn func() error) {
	if err := closeFn(); err != nil {
		log.Warn().Err(err).Msgf("failed to close %s", name)
	}
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

// scrapeDBPool periodically records the in-use DB connection count as a gauge.
func scrapeDBPool(ctx context.Context, db *postgres.DB) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics.DBPoolInUse.WithLabelValues("ppc").Set(float64(db.Stats().InUse))
		}
	}
}

// startServers starts the gRPC and HTTP servers and handles graceful shutdown.
func startServers(ctx context.Context, cfg *config.Config, ppcHandler *grpcdelivery.PPCHandler) error {
	// Setup gRPC server. Token blacklist and permissions reader are wired in a
	// later phase (shared auth Redis); nil is a safe graceful-degradation value.
	grpcServer, err := grpcdelivery.NewServer(&cfg.Server, &cfg.JWT, nil, nil)
	if err != nil {
		return err
	}

	// Register services.
	ppcv1.RegisterPPCServiceServer(grpcServer.GRPCServer(), ppcHandler)

	// Start gRPC server.
	go func() {
		if err := grpcServer.Start(); err != nil {
			log.Error().Err(err).Msg("gRPC server failed")
		}
	}()

	// Start HTTP gateway with CORS config.
	httpServer := httpdelivery.NewServer(&cfg.Server,
		httpdelivery.WithCORS(cfg.CORS.AllowedOrigins, cfg.CORS.MaxAge),
	)
	go func() {
		if err := httpServer.Start(ctx); err != nil {
			log.Warn().Err(err).Msg("HTTP server stopped")
		}
	}()

	// Wait for shutdown signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down servers...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	grpcServer.Stop()

	log.Info().Msg("Server shutdown complete")
	return nil
}
