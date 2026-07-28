package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/ppc/internal/application/etl"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/metrics"
)

// ProductionETL is the shared surface of a bobbin-production ETL usecase (TXT or
// SPG). Both return the same etl.Result shape (pulled/upserted/unmatched).
type ProductionETL interface {
	Run(ctx context.Context) (etl.Result, error)
}

// ProductionETLWorker runs a bobbin-production ETL on a fixed interval, logging
// and emitting metrics per run without ever crashing the process. The source
// label distinguishes TXT from SPG in logs and metrics.
type ProductionETLWorker struct {
	usecase  ProductionETL
	interval time.Duration
	source   string
}

// NewProductionETLWorker builds a production ETL worker with a metric source
// label. A non-positive interval defaults to 15 minutes.
func NewProductionETLWorker(usecase ProductionETL, interval time.Duration, source string) *ProductionETLWorker {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	return &ProductionETLWorker{usecase: usecase, interval: interval, source: source}
}

// Start runs an initial ETL pass then ticks on the configured interval until ctx
// is cancelled. Call in a goroutine: go worker.Start(ctx).
func (w *ProductionETLWorker) Start(ctx context.Context) {
	log.Info().Str("source", w.source).Dur("interval", w.interval).Msg("production ETL worker starting")

	w.runOnce(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info().Str("source", w.source).Msg("production ETL worker stopping")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

// runOnce executes a single ETL pass, recording metrics and logging the outcome.
// Errors are logged, not propagated (log-not-crash).
func (w *ProductionETLWorker) runOnce(ctx context.Context) {
	start := time.Now()
	res, err := w.usecase.Run(ctx)
	if err != nil {
		metrics.ETLErrorsTotal.WithLabelValues(w.source).Inc()
		metrics.ETLRunsTotal.WithLabelValues(w.source, metrics.StatusFailure).Inc()
		log.Error().Err(err).Str("source", w.source).Dur("elapsed", time.Since(start)).Msg("production ETL failed")
		return
	}

	metrics.ETLRunsTotal.WithLabelValues(w.source, metrics.StatusSuccess).Inc()
	metrics.ETLDurationSeconds.WithLabelValues(w.source).Observe(time.Since(start).Seconds())
	metrics.ETLRowsTotal.WithLabelValues(w.source, "upserted").Add(float64(res.Upserted))
	metrics.ETLRowsTotal.WithLabelValues(w.source, "unmatched").Add(float64(res.Unmatched))

	log.Info().
		Str("source", w.source).
		Int("pulled", res.Pulled).
		Int("upserted", res.Upserted).
		Int("unmatched", res.Unmatched).
		Bool("oracle_used", res.OracleUp).
		Dur("elapsed", time.Since(start)).
		Msg("production ETL completed")
}
