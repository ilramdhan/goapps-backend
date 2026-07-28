package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/ppc/internal/application/etl"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/metrics"
)

// etlSourceSoStaging is the metric source label for the SO staging ETL.
const etlSourceSoStaging = "so_staging"

// SoStagingETLWorker runs the full-replace sales-order staging ETL on a fixed
// interval. It logs and emits metrics on each run but never crashes the process.
type SoStagingETLWorker struct {
	usecase  *etl.SoStagingETL
	interval time.Duration
}

// NewSoStagingETLWorker builds the worker. A non-positive interval defaults to
// 30 minutes.
func NewSoStagingETLWorker(usecase *etl.SoStagingETL, interval time.Duration) *SoStagingETLWorker {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	return &SoStagingETLWorker{usecase: usecase, interval: interval}
}

// Start runs an initial ETL pass then ticks on the configured interval until ctx
// is cancelled. Call in a goroutine: go worker.Start(ctx).
func (w *SoStagingETLWorker) Start(ctx context.Context) {
	log.Info().Dur("interval", w.interval).Msg("so staging ETL worker starting")

	w.runOnce(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("so staging ETL worker stopping")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

// runOnce executes a single ETL pass, recording metrics and logging the outcome.
// Errors are logged, not propagated (log-not-crash).
func (w *SoStagingETLWorker) runOnce(ctx context.Context) {
	start := time.Now()
	res, err := w.usecase.Run(ctx)
	if err != nil {
		metrics.ETLErrorsTotal.WithLabelValues(etlSourceSoStaging).Inc()
		metrics.ETLRunsTotal.WithLabelValues(etlSourceSoStaging, metrics.StatusFailure).Inc()
		log.Error().Err(err).Dur("elapsed", time.Since(start)).Msg("so staging ETL failed")
		return
	}

	metrics.ETLRunsTotal.WithLabelValues(etlSourceSoStaging, metrics.StatusSuccess).Inc()
	metrics.ETLDurationSeconds.WithLabelValues(etlSourceSoStaging).Observe(time.Since(start).Seconds())
	metrics.ETLRowsTotal.WithLabelValues(etlSourceSoStaging, "replaced").Add(float64(res.Replaced))

	log.Info().
		Int("replaced", res.Replaced).
		Bool("oracle_used", res.OracleUp).
		Dur("elapsed", time.Since(start)).
		Msg("so staging ETL completed")
}
