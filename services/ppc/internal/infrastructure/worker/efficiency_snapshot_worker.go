package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	dpapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/dailyperf"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/metrics"
)

// snapshotWorkerName labels this worker in metrics and logs.
const snapshotWorkerName = "efficiency_snapshot"

// efficiencyAreas are the production areas recomputed on each safety pass.
var efficiencyAreas = []string{"TXT", "SPG", "TWT"}

// EfficiencySnapshotWorker periodically recomputes today's efficiency snapshots
// across all areas as a safety net behind on-demand recomputes. It logs and emits
// metrics on each run and never crashes the process on error.
type EfficiencySnapshotWorker struct {
	svc      *dpapp.Service
	interval time.Duration
}

// NewEfficiencySnapshotWorker builds the worker. A non-positive interval defaults
// to 15 minutes.
func NewEfficiencySnapshotWorker(svc *dpapp.Service, interval time.Duration) *EfficiencySnapshotWorker {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	return &EfficiencySnapshotWorker{svc: svc, interval: interval}
}

// Start runs an initial recompute then ticks on the configured interval until ctx
// is cancelled. Call in a goroutine: go worker.Start(ctx). A nil service is a
// no-op (the worker returns immediately).
func (w *EfficiencySnapshotWorker) Start(ctx context.Context) {
	if w.svc == nil {
		log.Info().Msg("efficiency snapshot worker disabled (nil service)")
		return
	}
	log.Info().Dur("interval", w.interval).Msg("efficiency snapshot worker starting")

	w.runOnce(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("efficiency snapshot worker stopping")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

// runOnce recomputes today's snapshots for every area, logging outcome and
// recording metrics. Per-area errors are logged, not propagated (log-not-crash).
func (w *EfficiencySnapshotWorker) runOnce(ctx context.Context) {
	start := time.Now()
	date := time.Now().Truncate(24 * time.Hour)

	var totalWritten int
	var failed bool
	for _, areaCode := range efficiencyAreas {
		written, err := w.svc.Recalc(ctx, areaCode, date, nil, nil)
		if err != nil {
			failed = true
			log.Error().Err(err).Str("area", areaCode).Msg("efficiency snapshot recompute failed")
			continue
		}
		totalWritten += written
		metrics.EfficiencySnapshotsWritten.WithLabelValues(areaCode).Add(float64(written))
	}

	status := metrics.StatusSuccess
	if failed {
		status = metrics.StatusFailure
	}
	metrics.WorkerRunsTotal.WithLabelValues(snapshotWorkerName, status).Inc()

	log.Info().
		Int("snapshots_written", totalWritten).
		Bool("had_failures", failed).
		Dur("elapsed", time.Since(start)).
		Msg("efficiency snapshot recompute completed")
}
