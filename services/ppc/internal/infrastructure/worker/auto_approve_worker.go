package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	workorderapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/workorder"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/metrics"
)

// Clock supplies the current time; injectable so tests can drive auto-approve
// timing deterministically.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// AutoApproveWorker periodically auto-approves WOs whose PC/PM approval has been
// pending longer than the configured window. It logs and emits metrics on each
// run but never crashes the process on error (log-not-crash).
type AutoApproveWorker struct {
	svc      *workorderapp.Service
	window   time.Duration
	interval time.Duration
	clock    Clock
}

// NewAutoApproveWorker builds the worker. A non-positive window defaults to 4h;
// a non-positive interval defaults to 1 minute.
func NewAutoApproveWorker(svc *workorderapp.Service, autoApproveHours int, interval time.Duration) *AutoApproveWorker {
	window := time.Duration(autoApproveHours) * time.Hour
	if window <= 0 {
		window = 4 * time.Hour
	}
	if interval <= 0 {
		interval = time.Minute
	}
	return &AutoApproveWorker{svc: svc, window: window, interval: interval, clock: realClock{}}
}

// WithClock overrides the worker clock (tests). Returns the worker for chaining.
func (w *AutoApproveWorker) WithClock(c Clock) *AutoApproveWorker {
	w.clock = c
	return w
}

// Start runs an initial sweep then ticks on the configured interval until ctx is
// cancelled. Call in a goroutine: go worker.Start(ctx).
func (w *AutoApproveWorker) Start(ctx context.Context) {
	log.Info().Dur("interval", w.interval).Dur("window", w.window).Msg("auto-approve worker starting")

	w.runOnce(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("auto-approve worker stopping")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

// runOnce executes a single auto-approve sweep, logging outcome + metrics.
func (w *AutoApproveWorker) runOnce(ctx context.Context) {
	start := time.Now()
	res, err := w.svc.AutoApprovePending(ctx, w.clock.Now(), w.window)
	if err != nil {
		metrics.WOAutoApproveRunsTotal.WithLabelValues(metrics.StatusFailure).Inc()
		log.Error().Err(err).Dur("elapsed", time.Since(start)).Msg("auto-approve sweep failed")
		return
	}

	metrics.WOAutoApproveRunsTotal.WithLabelValues(metrics.StatusSuccess).Inc()
	metrics.WOAutoApproveActionsTotal.WithLabelValues("pc").Add(float64(res.PCApproved))
	metrics.WOAutoApproveActionsTotal.WithLabelValues("pm").Add(float64(res.PMApproved))
	metrics.WOAutoApproveActionsTotal.WithLabelValues("approved").Add(float64(res.FullyApproved))

	if res.PCApproved > 0 || res.PMApproved > 0 {
		log.Info().
			Int("scanned", res.Scanned).
			Int("pc_approved", res.PCApproved).
			Int("pm_approved", res.PMApproved).
			Int("fully_approved", res.FullyApproved).
			Dur("elapsed", time.Since(start)).
			Msg("auto-approve sweep completed")
	}
}
