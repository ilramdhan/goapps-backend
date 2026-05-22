// Package worker contains the runtime loop that consumes chunk messages from
// RabbitMQ and executes calc batches. The bootstrap exposes only the lifecycle
// skeleton; real consume + calc + publish logic lands in S8c.7.
package worker

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance-cost-worker/internal/config"
	"github.com/mutugading/goapps-backend/services/finance-cost-worker/internal/infrastructure/rmq"
)

// Worker is the top-level worker runtime. Wired in main and started as a
// goroutine.
type Worker struct {
	cfg      *config.Config
	rmqConn  *rmq.Connection
	workerID string
}

// New constructs a Worker with its required dependencies. Additional deps
// (DB repos, calc engine, publisher) will be injected in S8c.7.
func New(cfg *config.Config, rmqConn *rmq.Connection, workerID string) *Worker {
	return &Worker{cfg: cfg, rmqConn: rmqConn, workerID: workerID}
}

// Run blocks until the context is cancelled. In S8c.7 this will start the
// consumer loop; for the bootstrap it just idles so the binary stays up and
// signal handling can shut it down cleanly.
func (w *Worker) Run(ctx context.Context) error {
	log.Info().
		Str("worker_id", w.workerID).
		Int("prefetch_count", w.cfg.RabbitMQ.PrefetchCount).
		Msg("Worker started (bootstrap — no work yet)")

	<-ctx.Done()

	log.Info().Str("worker_id", w.workerID).Msg("Worker stopping")
	return nil
}
