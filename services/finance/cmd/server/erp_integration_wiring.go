package main

import (
	"github.com/rs/zerolog/log"

	auditapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costauditlog"
	cpmapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	erpapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	erpruleapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erprule"
	periodlockapp "github.com/mutugading/goapps-backend/services/finance/internal/application/periodlock"
	grpcdelivery "github.com/mutugading/goapps-backend/services/finance/internal/delivery/grpc"
	cptdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
	domainerp "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	domainperiodlock "github.com/mutugading/goapps-backend/services/finance/internal/domain/periodlock"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
)

// erpIntegrationHandlers groups the ERP-integration application handlers
// built by the server. They get their RPC surface in P6; until then they are
// wired (so DI failures surface at boot) but not yet exposed.
type erpIntegrationHandlers struct {
	// batches is the read side of the batch repository (P6-T3a Get/List RPCs).
	batches     *postgres.ErpIntBatchRepository
	createBatch *erpapp.CreateBatchHandler
	stepTrigger *erpapp.StepTriggerHandler
	lock        *periodlockapp.LockHandler
	unlock      *periodlockapp.UnlockHandler
	// attrBackfill is the P3-T6 one-off attribute backfill (fill-NULL only;
	// ErrAttrBackfillNotConfigured when Oracle is absent).
	attrBackfill *erpapp.BackfillAttributesHandler
	// linkReadiness is the P3-T9 read-only link-readiness report (§9a–§9f).
	linkReadiness *erpapp.LinkReadinessHandler
	// ackWarnings is the P5-T2 V-05 / V-08w warnings ack (ERP_WARN_ACK; the
	// RBAC check is in delivery, P6-T3).
	ackWarnings *erpapp.AckWarningsHandler
	// abandon is the AbandonErpBatch handler (PG only).
	abandon *erpapp.AbandonBatchHandler
	// pushPreview is the P5-T3 read-only push preview (no Oracle call); the
	// push itself is stepTrigger.TriggerPush -> worker PushStep.
	pushPreview *erpapp.PushPreviewHandler
	// valuationPreview is the P5-T4 VALUATE preview (read-only: SELECT on
	// Oracle through the ReadOnlyGuard; the execute is stepTrigger.
	// TriggerAdjExecute -> worker AdjExecuteStep, P5-T5).
	valuationPreview *erpapp.ValuationPreviewHandler
	// scheduleHandler is the P5-T11 schedule view/update. The scheduler
	// itself runs on the worker (singleton), so no reloader is set here; the
	// worker re-resolves the schedule at start-up.
	scheduleHandler *erpapp.ScheduleHandler
	// T3b read-side collaborators of the gRPC handler.
	previews      *postgres.ErpValuationPreviewRepository
	coverage      *postgres.ErpCoverageRepository
	linkErp       *grpcdelivery.ErpProductLinker
	createProduct *erpapp.CreateProductFromDemand
	masterSync    *erpapp.MasterSyncTriggerHandler
	stdCost       *postgres.ErpStdCostRepository
	oracleCall    *postgres.ErpOracleCallRepository
	backtest      *postgres.ErpBacktestReportRepository
	sanity        *erpapp.CurrencySanityService
	cfgView       grpcdelivery.ErpConfigView
}

// setupErpIntegration wires the plan-04 P3 handlers. Oracle is reached only
// through the ReadOnlyGuard querier (SELECT only). When Oracle is absent the
// ADJ probe stays nil, which keeps both the period unlock (P3-T3,
// ErrAdjProbeNotConfigured) and LIVE CreateBatch (V-10) fail-closed. A nil
// RabbitMQ adapter becomes a true nil publisher (ErrPublisherUnavailable).
func setupErpIntegration(
	cfg *config.Config,
	db *postgres.DB,
	jobRepo *postgres.JobRepository,
	periodLockRepo *postgres.PeriodLockRepository,
	auditRepo *postgres.CostAuditLogRepository,
	oracleClient *oracleinfra.Client,
	oracleErr error,
	rmqAdapter *rabbitmq.JobPublisherAdapter,
	writer erpapp.WriterGate,
	cpmRepo *postgres.CostProductMasterRepository,
	cptRepo cptdomain.Repository,
) erpIntegrationHandlers {
	var unlockProbe domainperiodlock.AdjPostedProbe
	var headProber domainerp.ErpAdjHeadProber
	// legacyReader stays a true nil interface without Oracle, so the backfill
	// fails closed with ErrAttrBackfillNotConfigured. FG_PRD_PER_DAY is not
	// read (WithPrdPerDay unset: column unconfirmed).
	var legacyReader domainerp.LegacyStdReader
	// snapshotReader stays a true nil interface without Oracle, so the
	// valuation preview fails closed (ErrAdjSnapshotReaderNotConfigured).
	var snapshotReader domainerp.AdjSnapshotReader
	if oracleErr == nil && oracleClient != nil {
		snapshotReader = oracleinfra.NewErpAdjSnapshotReader(oracleClient.ReadOnly())
		adjReader := oracleinfra.NewErpAdjReader(oracleClient.ReadOnly())
		unlockProbe = oracleinfra.NewAdjPostedProbe(adjReader)
		headProber = adjReader
		legacyReader = oracleinfra.NewLegacyStdReader(oracleClient.ReadOnly())
	} else {
		log.Warn().Msg("Oracle unavailable; period unlock and LIVE ERP batch create fail closed (no ADJ posted probe)")
	}
	var publisher erpapp.ErpJobPublisher
	var syncPublisher erpapp.ErpMasterSyncPublisher
	if rmqAdapter != nil {
		publisher = rmqAdapter
		syncPublisher = rmqAdapter
	}
	emitter := auditapp.NewEmitter(auditRepo)
	linkHandler := cpmapp.NewLinkErpItemHandler(cpmRepo, cpmRepo, cptRepo, emitter)
	productCreator := grpcdelivery.NewErpProductCreator(cpmapp.NewCreateHandler(cpmRepo, cptRepo), linkHandler)
	batchRepo := postgres.NewErpIntBatchRepository(db)
	return erpIntegrationHandlers{
		previews:      postgres.NewErpValuationPreviewRepository(db),
		coverage:      postgres.NewErpCoverageRepository(db),
		linkErp:       grpcdelivery.NewErpProductLinker(linkHandler, cpmRepo),
		createProduct: erpapp.NewCreateProductFromDemand(postgres.NewErpCoverageRepository(db), productCreator, productCreator, cptRepo, nil),
		masterSync:    erpapp.NewMasterSyncTriggerHandler(jobRepo, syncPublisher),
		stdCost:       postgres.NewErpStdCostRepository(db),
		oracleCall:    postgres.NewErpOracleCallRepository(db),
		backtest:      postgres.NewErpBacktestReportRepository(db),
		sanity:        erpapp.NewCurrencySanityService(postgres.NewCurrencySanityRepository(db)),
		cfgView: grpcdelivery.ErpConfigView{
			PushEnabled: cfg.ERP.PushEnabled, ValuationEnabled: cfg.ERP.ValuationEnabled,
			AdjApproveEnabled: cfg.ERP.AdjApproveEnabled, WriterMode: string(writer.Mode),
			OracleIFConfigured: cfg.OracleIF.Host != "",
		},
		batches:     batchRepo,
		createBatch: erpapp.NewCreateBatchHandler(batchRepo, periodLockRepo, headProber),
		stepTrigger: erpapp.NewStepTriggerHandler(jobRepo, batchRepo, publisher, cfg.ERP.DemandMaxAge).
			WithPushGates(cfg.ERP.PushEnabled, writer).
			WithAdjGates(cfg.ERP.ValuationEnabled, cfg.ERP.AdjApproveEnabled, writer),
		lock:   periodlockapp.NewLockHandler(periodLockRepo, emitter),
		unlock: periodlockapp.NewUnlockHandler(periodLockRepo, unlockProbe, emitter),
		attrBackfill: erpapp.NewBackfillAttributesHandler(legacyReader,
			postgres.NewErpAttrBackfillRepository(db), emitter),
		linkReadiness: erpapp.NewLinkReadinessHandler(batchRepo,
			postgres.NewErpDemandRepository(db), postgres.NewErpLinkReadinessRepository(db)),
		abandon:     erpapp.NewAbandonBatchHandler(postgres.NewErpBatchTxRunner(db)),
		ackWarnings: erpapp.NewAckWarningsHandler(postgres.NewErpBatchTxRunner(db), emitter),
		pushPreview: erpapp.NewPushPreviewHandler(postgres.NewErpBatchTxRunner(db), cfg.ERP.PushEnabled, writer.Mode),
		valuationPreview: erpapp.NewValuationPreviewHandler(batchRepo, periodLockRepo, snapshotReader,
			postgres.NewErpStdCostRepository(db), postgres.NewErpValuationPreviewRepository(db),
			erpapp.ValuationPreviewConfig{
				ValuationEnabled: cfg.ERP.ValuationEnabled, WriterMode: writer.Mode, TTL: cfg.ERP.PreviewTTL,
			}),
		scheduleHandler: erpapp.NewScheduleHandler(postgres.NewErpIntegrationSettingRepository(db),
			erpapp.ScheduleConfig(cfg.ERP.Schedule), emitter),
	}
}

// setupErpWriter builds the ERP Oracle writer through the fail-closed factory
// and logs the effective mode. The push trigger (P5-T3) checks it as G2
// before enqueueing; the worker builds its own writer for the push itself.
// A factory error is logged and never aborts start-up — the writer is
// disabled then.
func setupErpWriter(cfg *config.Config) erpapp.WriterGate {
	w, erpMode, werr := oracleinfra.NewErpWriter(cfg.ERP, cfg.OracleIF, cfg.App.Env, log.Logger)
	if werr != nil {
		log.Warn().Err(werr).Str("erp_writer_mode", string(erpMode)).Msg("ERP writer not available; running disabled")
		return erpapp.WriterGate{Writer: w, Mode: erpMode}
	}
	log.Info().Str("erp_writer_mode", string(erpMode)).Msg("ERP writer configured")
	return erpapp.WriterGate{Writer: w, Mode: erpMode}
}

// setupErpRule builds the ERP rule master RPC handler (P6-T3c).
func setupErpRule(db *postgres.DB, batches *postgres.ErpIntBatchRepository, auditRepo *postgres.CostAuditLogRepository) *grpcdelivery.ErpRuleHandler {
	rules := postgres.NewErpVallossRuleRepository(db)
	prices := postgres.NewErpSellPriceRepository(db)
	grades := postgres.NewErpGradeGroupRepository(db)
	loader := postgres.NewErpRuleSetLoader(db)
	emitter := auditapp.NewEmitter(auditRepo)
	return grpcdelivery.NewErpRuleGRPCHandler(grpcdelivery.ErpRuleDeps{
		List:        erpruleapp.NewListVallossRulesHandler(rules),
		Create:      erpruleapp.NewCreateVallossRuleHandler(rules, emitter),
		Update:      erpruleapp.NewUpdateVallossRuleHandler(rules, emitter),
		Rules:       rules,
		Delete:      erpruleapp.NewDeleteVallossRuleHandler(rules, loader, loader, emitter),
		ListPrices:  erpruleapp.NewListSellPricesHandler(prices),
		UpsertPrice: erpruleapp.NewUpsertSellPriceHandler(prices, emitter),
		ListGrades:  erpruleapp.NewListGradeGroupsHandler(grades),
		AssignGrade: erpruleapp.NewAssignGradeGroupHandler(grades, emitter),
		Batches:     batches,
		Export:      erpruleapp.NewExportRulesHandler(rules, prices, grades, loader),
	})
}
