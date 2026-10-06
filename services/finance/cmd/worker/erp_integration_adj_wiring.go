package main

import (
	"context"

	"github.com/rs/zerolog/log"

	auditapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costauditlog"
	erpapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	erpdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	erpmetrics "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/metrics"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
)

// wireErpAdjSteps attaches the W2 steps (plan-06 P5-T5) and the recon
// read-back step (P5-T6). Oracle is changed only through the OracleWriter
// package calls (PKG_GOAPPS_ADJ); the ADJ rows and the recon read-back go
// through the ReadOnlyGuard querier (SELECT only). Without Oracle
// the snapshot / recon readers and the posted probe stay true nil interfaces,
// so the steps fail closed before any call. Flags default off and the writer
// defaults to disabled (G1/G2).
func wireErpAdjSteps(exec *erpapp.JobExecutor, cfg *config.Config, db *postgres.DB, runner *postgres.ErpBatchTxRunner,
	gate erpapp.WriterGate, oracleClient *oracle.Client, prober erpdomain.ErpAdjHeadProber,
) {
	var (
		reader erpdomain.AdjSnapshotReader
		recon  erpdomain.ErpReconReader
	)
	if oracleClient != nil {
		reader = oracle.NewErpAdjSnapshotReader(oracleClient.ReadOnly())
		recon = oracle.NewErpReconReader(oracleClient.ReadOnly())
	}
	callRepo := postgres.NewErpOracleCallRepository(db)
	calls := erpmetrics.NewInstrumentedCallLog(callRepo)
	audit := auditapp.NewEmitter(postgres.NewCostAuditLogRepository(db))
	previews := postgres.NewErpValuationPreviewRepository(db)
	adj := erpapp.NewAdjExecuteStep(runner, gate, calls, audit, previews, previews, reader, erpapp.AdjExecuteConfig{
		ValuationEnabled: cfg.ERP.ValuationEnabled, AdjApproveEnabled: cfg.ERP.AdjApproveEnabled,
		CallTimeout: cfg.ERP.CallTimeout, BusyRetries: cfg.ERP.BusyRetries,
	})
	lock := erpapp.NewLockBatchStep(runner, gate, calls, audit, prober, cfg.ERP.ValuationEnabled,
		cfg.ERP.CallTimeout, cfg.ERP.BusyRetries)
	// Recon (P5-T6) is SELECT-only towards Oracle; it settles STARTED /
	// UNKNOWN call-log rows from the read-back and never calls the writer.
	exec.WithAdjExecute(adj).WithLockBatch(lock).WithRecon(erpapp.NewReconStep(runner, recon, callRepo))
}

// wireErpScheduler builds the scheduled read/compute chain (plan-06 P5-T11)
// on the worker (a singleton deployment) and starts it. It never enqueues
// push or any W2 step. Without a publisher the scheduler stays off.
func wireErpScheduler(ctx context.Context, exec *erpapp.JobExecutor, cfg *config.Config, db *postgres.DB,
	jobRepo *postgres.JobRepository, prober erpdomain.ErpAdjHeadProber, publisher *rabbitmq.JobPublisherAdapter,
) func() {
	if publisher == nil {
		log.Warn().Msg("erp schedule: no job publisher; scheduler off")
		return func() {}
	}
	batches := postgres.NewErpIntBatchRepository(db)
	create := erpapp.NewCreateBatchHandler(batches, postgres.NewPeriodLockRepository(db), prober)
	steps := erpapp.NewStepTriggerHandler(jobRepo, batches, publisher, cfg.ERP.DemandMaxAge)
	runner := erpapp.NewScheduleRunner(batches, create, steps)
	exec.WithScheduleRunner(runner)
	sched := erpapp.NewErpScheduler(erpScheduleConfig(cfg), postgres.NewErpIntegrationSettingRepository(db), runner)
	res := sched.Start(ctx)
	log.Info().Bool("enabled", res.Enabled).Str("source", res.Source).Msg("erp schedule started")
	return sched.Stop
}

// erpScheduleConfig maps the YAML schedule block to the resolver input.
func erpScheduleConfig(cfg *config.Config) erpapp.ScheduleConfig {
	return erpapp.ScheduleConfig(cfg.ERP.Schedule)
}
