// Package worker provides background workers for the PPC service.
package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/ppc/internal/application/machinesync"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/metrics"
)

// MachineSyncWorker runs the machine sync on a fixed interval. It logs and emits
// metrics on each run but never crashes the process on a sync error.
type MachineSyncWorker struct {
	usecase  *machinesync.Usecase
	interval time.Duration
}

// NewMachineSyncWorker builds the worker. A non-positive interval defaults to 24h.
func NewMachineSyncWorker(usecase *machinesync.Usecase, interval time.Duration) *MachineSyncWorker {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &MachineSyncWorker{usecase: usecase, interval: interval}
}

// Start runs an initial sync then ticks on the configured interval until ctx is
// cancelled. Call in a goroutine: go worker.Start(ctx).
func (w *MachineSyncWorker) Start(ctx context.Context) {
	log.Info().Dur("interval", w.interval).Msg("machine sync worker starting")

	w.runOnce(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("machine sync worker stopping")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

// runOnce executes a single sync, logging outcome and recording metrics. Errors
// are logged, not propagated (log-not-crash).
func (w *MachineSyncWorker) runOnce(ctx context.Context) {
	start := time.Now()
	res, err := w.usecase.Sync(ctx)
	if err != nil {
		metrics.MachineSyncRunsTotal.WithLabelValues(metrics.StatusFailure).Inc()
		log.Error().Err(err).Dur("elapsed", time.Since(start)).Msg("machine sync failed")
		return
	}

	metrics.MachineSyncRunsTotal.WithLabelValues(metrics.StatusSuccess).Inc()
	metrics.MachineSyncUpsertsTotal.WithLabelValues("inserted").Add(float64(res.Inserted))
	metrics.MachineSyncUpsertsTotal.WithLabelValues("updated").Add(float64(res.Updated))
	metrics.MachineSyncUpsertsTotal.WithLabelValues("skipped").Add(float64(res.Skipped))

	log.Info().
		Int("inserted", res.Inserted).
		Int("updated", res.Updated).
		Int("skipped", res.Skipped).
		Bool("finance_used", res.FinanceUsed).
		Bool("oracle_used", res.OracleUsed).
		Dur("elapsed", time.Since(start)).
		Msg("machine sync completed")
}
