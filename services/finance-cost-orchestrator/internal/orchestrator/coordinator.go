// Package orchestrator coordinates calc-job execution: planning chunks,
// publishing them to worker queues, consuming chunk-done events, and
// finalizing the job.
package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/mutugading/goapps-backend/pkg/costcalc"
	"github.com/mutugading/goapps-backend/pkg/costcalc/metrics"
	"github.com/mutugading/goapps-backend/services/finance-cost-orchestrator/internal/config"
	"github.com/mutugading/goapps-backend/services/finance-cost-orchestrator/internal/infrastructure/rmq"
)

// Coordinator is the top-level orchestrator runtime. Wired in main and started
// as a goroutine.
type Coordinator struct {
	cfg         *config.Config
	rmqConn     *rmq.Connection
	pub         *rmq.Publisher
	dag         *DagBuilder
	jobRepo     *JobRepo
	chunkRepo   *ChunkRepo
	productRepo *JobProductRepo
}

// New constructs a Coordinator with its required dependencies.
func New(cfg *config.Config, rmqConn *rmq.Connection, db *sql.DB) *Coordinator {
	return &Coordinator{
		cfg:         cfg,
		rmqConn:     rmqConn,
		pub:         rmq.NewPublisher(rmqConn),
		dag:         NewDagBuilder(db),
		jobRepo:     NewJobRepo(db),
		chunkRepo:   NewChunkRepo(db),
		productRepo: NewJobProductRepo(db),
	}
}

// Run starts two RMQ consumers (job_triggered + chunk_completed) and blocks
// until ctx is canceled or one of them fails.
func (c *Coordinator) Run(ctx context.Context) error {
	log.Info().
		Int("chunk_size", c.cfg.Orchestrator.ChunkSize).
		Int("max_chunk_size", c.cfg.Orchestrator.MaxChunkSize).
		Msg("Orchestrator coordinator started")

	jobConsumer := rmq.NewConsumer(c.rmqConn, rmq.QueueJobTriggered, "orchestrator-job-triggered")
	chunkConsumer := rmq.NewConsumer(c.rmqConn, rmq.QueueChunkDone, "orchestrator-chunk-completed")

	go c.scrapeQueueDepth(ctx)

	errCh := make(chan error, 2)
	go func() { errCh <- jobConsumer.Consume(ctx, c.handleJobTriggered) }()
	go func() { errCh <- chunkConsumer.Consume(ctx, c.handleChunkCompleted) }()

	select {
	case <-ctx.Done():
		log.Info().Msg("Orchestrator coordinator stopping (ctx cancelled)")
		return nil
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("coordinator consumer exit: %w", err)
		}
		return nil
	}
}

// handleJobTriggered processes a JobTriggeredEvent: plans the DAG, packs
// chunks, persists rows, dispatches wave 0.
func (c *Coordinator) handleJobTriggered(ctx context.Context, d amqp.Delivery) error {
	var ev JobTriggeredEvent
	if err := json.Unmarshal(d.Body, &ev); err != nil {
		log.Error().Err(err).Bytes("body", d.Body).Msg("malformed job_triggered; nacking to DLQ")
		return d.Nack(false, false)
	}
	if err := c.planAndDispatch(ctx, ev.JobID); err != nil {
		log.Error().Err(err).Int64("job_id", ev.JobID).Msg("plan and dispatch failed")
		if updErr := c.jobRepo.UpdateStatus(ctx, ev.JobID, statusFailed); updErr != nil {
			log.Warn().Err(updErr).Int64("job_id", ev.JobID).Msg("mark job FAILED")
		}
		c.emitJobTerminal(ctx, ev.JobID, statusFailed)
	}
	return d.Ack(false)
}

// planAndDispatch is the full job-bootstrap pipeline. Each step is sequential
// and aborts on first error; partial state is left for ops to inspect.
func (c *Coordinator) planAndDispatch(ctx context.Context, jobID int64) error {
	ctx, span := otel.Tracer(tracerName).Start(ctx, spanCostCalcJob, trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	job, err := c.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("get job: %w", err)
	}
	if job.Status != statusQueued {
		log.Warn().Int64("job_id", jobID).Str("status", job.Status).Msg("duplicate job_triggered; ignoring")
		return nil
	}
	span.SetAttributes(
		attribute.Int64("job_id", jobID),
		attribute.String("job_code", job.JobCode),
		attribute.String("period", job.Period),
		attribute.String("calc_type", string(job.CalcType)),
		attribute.String("scope", string(job.Scope)),
	)
	if err := c.jobRepo.UpdateStatus(ctx, jobID, statusPlanning); err != nil {
		return fmt.Errorf("update status PLANNING: %w", err)
	}

	graph, productIDs, err := c.dag.Build(ctx, ScopeInput{
		Scope:               job.Scope,
		ProductSysID:        job.ProductSysID,
		RouteHeadID:         job.RouteHeadID,
		ProductTypeIDFilter: job.ProductTypeIDFilter,
		Period:              job.Period,
	})
	if err != nil {
		return fmt.Errorf("dag build: %w", err)
	}
	if len(productIDs) == 0 {
		log.Info().Int64("job_id", jobID).Msg("no products in scope; completing as SUCCESS")
		c.emitJobTerminal(ctx, jobID, statusSuccess)
		return c.jobRepo.CompleteJob(ctx, jobID, statusSuccess, 0, 0, 0, 0)
	}

	plan := costcalc.PlanWaves(graph)
	if len(plan.Cyclic) > 0 {
		log.Error().Int64("job_id", jobID).Ints64("cyclic", plan.Cyclic).Msg("dependency cycle detected; failing job")
		c.emitJobTerminal(ctx, jobID, statusFailed)
		return c.jobRepo.CompleteJob(ctx, jobID, statusFailed, 0, 0, len(plan.Cyclic), 0)
	}

	waves := PackChunks(plan, c.cfg.Orchestrator.ChunkSize, c.cfg.Orchestrator.MaxChunkSize)
	FillJobContext(waves, jobID, job.JobCode, job.Period, job.CreatedBy, job.CalcType)

	routeMap, err := c.productRepo.ResolveProductRouteMap(ctx, productIDs)
	if err != nil {
		return fmt.Errorf("resolve route map: %w", err)
	}

	totalChunks, err := c.persistWavePlan(ctx, jobID, waves, routeMap)
	if err != nil {
		return fmt.Errorf("persist wave plan: %w", err)
	}

	span.SetAttributes(
		attribute.Int("total_products", len(productIDs)),
		attribute.Int("total_chunks", totalChunks),
	)

	if err := c.jobRepo.UpdateTotals(ctx, jobID, len(productIDs), totalChunks, len(waves)); err != nil {
		return fmt.Errorf("update totals: %w", err)
	}
	if err := c.jobRepo.MarkStarted(ctx, jobID); err != nil {
		return fmt.Errorf("mark started: %w", err)
	}
	if err := c.jobRepo.UpdateStatus(ctx, jobID, statusProcessing); err != nil {
		return fmt.Errorf("update status PROCESSING: %w", err)
	}

	if len(waves) == 0 {
		// Defensive: PackChunks produced nothing for non-empty product set —
		// finalize empty so the job doesn't hang forever.
		c.emitJobTerminal(ctx, jobID, statusSuccess)
		return c.jobRepo.CompleteJob(ctx, jobID, statusSuccess, 0, 0, 0, 0)
	}
	return c.dispatchWave(ctx, waves[0].Chunks)
}

// persistWavePlan bulk-inserts chunks (collecting IDs), then bulk-inserts
// job_products with the resolved chunk_id + route_head_id.
func (c *Coordinator) persistWavePlan(
	ctx context.Context,
	jobID int64,
	waves []PackedWave,
	routeMap map[int64]int64,
) (int, error) {
	totalChunks := 0
	for waveIdx := range waves {
		wave := &waves[waveIdx]
		chunkRows := make([]*ChunkRow, 0, len(wave.Chunks))
		for i := range wave.Chunks {
			chunkRows = append(chunkRows, &ChunkRow{
				JobID:       jobID,
				ChunkNumber: wave.Chunks[i].ChunkNumber,
				WaveNo:      wave.Chunks[i].WaveNo,
				ProductIDs:  wave.Chunks[i].ProductIDs,
			})
		}
		if err := c.chunkRepo.BulkInsert(ctx, chunkRows); err != nil {
			return 0, fmt.Errorf("bulk insert chunks (wave %d): %w", wave.Number, err)
		}
		// Wire chunk IDs back to the in-memory ChunkSpec so dispatchWave +
		// downstream worker have them.
		productRows := make([]*JobProductRow, 0)
		for i := range wave.Chunks {
			wave.Chunks[i].ChunkID = chunkRows[i].ChunkID
			for _, pid := range wave.Chunks[i].ProductIDs {
				productRows = append(productRows, &JobProductRow{
					JobID:        jobID,
					ProductSysID: pid,
					RouteHeadID:  routeMap[pid],
					WaveNo:       wave.Chunks[i].WaveNo,
					ChunkID:      chunkRows[i].ChunkID,
				})
			}
		}
		if err := c.productRepo.BulkInsert(ctx, productRows); err != nil {
			return 0, fmt.Errorf("bulk insert job_products (wave %d): %w", wave.Number, err)
		}
		totalChunks += len(chunkRows)
	}
	return totalChunks, nil
}

// dispatchWave publishes each ChunkSpec to the worker queue and flips the
// chunk row to DISPATCHED.
func (c *Coordinator) dispatchWave(ctx context.Context, chunks []ChunkSpec) error {
	for i := range chunks {
		if err := c.pub.Publish(ctx, rmq.RoutingKeyChunk, chunks[i]); err != nil {
			return fmt.Errorf("publish chunk %d: %w", chunks[i].ChunkID, err)
		}
		if err := c.chunkRepo.UpdateDispatched(ctx, chunks[i].ChunkID); err != nil {
			return fmt.Errorf("mark chunk %d dispatched: %w", chunks[i].ChunkID, err)
		}
	}
	return nil
}

// handleChunkCompleted updates progress, dispatches the next wave when the
// current one is fully done, or finalizes the job on the last wave.
func (c *Coordinator) handleChunkCompleted(ctx context.Context, d amqp.Delivery) error {
	var ev ChunkCompletedEvent
	if err := json.Unmarshal(d.Body, &ev); err != nil {
		log.Error().Err(err).Bytes("body", d.Body).Msg("malformed chunk_completed; nacking to DLQ")
		return d.Nack(false, false)
	}
	if err := c.advanceAfterChunk(ctx, ev); err != nil {
		log.Error().Err(err).Int64("job_id", ev.JobID).Int64("chunk_id", ev.ChunkID).Msg("advance after chunk failed; requeuing")
		return d.Nack(false, true)
	}
	return d.Ack(false)
}

func (c *Coordinator) advanceAfterChunk(ctx context.Context, ev ChunkCompletedEvent) error {
	if err := c.jobRepo.IncrementProgress(ctx, ev.JobID, ev.SuccessCount, ev.FailedCount, ev.BlockedCount); err != nil {
		return fmt.Errorf("increment progress: %w", err)
	}
	total, completed, err := c.chunkRepo.CountByJobWave(ctx, ev.JobID, ev.WaveNo)
	if err != nil {
		return fmt.Errorf("count wave chunks: %w", err)
	}
	if completed < total {
		return nil // wave still in flight
	}

	// If the job was cancelled mid-flight (CancelJob RPC on finance set the
	// terminal status while this wave was running), stop dispatching: mark
	// remaining QUEUED chunks in later waves FAILED, and let cancel_job_handler's
	// terminal status stand without finalizing again.
	job, err := c.jobRepo.GetByID(ctx, ev.JobID)
	if err != nil {
		return fmt.Errorf("get job for cancel check: %w", err)
	}
	if job.Status == statusCancelled {
		if err := c.chunkRepo.MarkRemainingChunksSkipped(ctx, ev.JobID, ev.WaveNo+1); err != nil {
			log.Error().Err(err).Int64("job_id", ev.JobID).Msg("mark remaining chunks skipped")
			return fmt.Errorf("mark remaining skipped: %w", err)
		}
		log.Info().Int64("job_id", ev.JobID).Int("from_wave", ev.WaveNo+1).Msg("job cancelled mid-flight; remaining waves skipped")
		return nil
	}

	totalWaves, err := c.getTotalWaves(ctx, ev.JobID)
	if err != nil {
		return fmt.Errorf("get total waves: %w", err)
	}
	if ev.WaveNo+1 < totalWaves {
		return c.dispatchNextWave(ctx, ev.JobID, ev.WaveNo+1)
	}
	return c.finalizeJob(ctx, ev.JobID)
}

// dispatchNextWave loads next wave's persisted chunks and re-publishes them as
// ChunkSpec messages. The wave already has chunk rows from planning.
func (c *Coordinator) dispatchNextWave(ctx context.Context, jobID int64, waveNo int) error {
	job, err := c.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("get job for next wave: %w", err)
	}
	rows, err := c.chunkRepo.ListChunksOfWave(ctx, jobID, waveNo)
	if err != nil {
		return fmt.Errorf("list chunks of wave %d: %w", waveNo, err)
	}
	chunks := make([]ChunkSpec, 0, len(rows))
	for _, r := range rows {
		chunks = append(chunks, ChunkSpec{
			JobID:       jobID,
			JobCode:     job.JobCode,
			ChunkID:     r.ChunkID,
			ChunkNumber: r.ChunkNumber,
			WaveNo:      r.WaveNo,
			Period:      job.Period,
			CalcType:    job.CalcType,
			ProductIDs:  r.ProductIDs,
			Actor:       job.CreatedBy,
		})
	}
	return c.dispatchWave(ctx, chunks)
}

// finalizeJob computes terminal status + duration and writes them.
func (c *Coordinator) finalizeJob(ctx context.Context, jobID int64) error {
	_, _, succ, fail, blocked, err := c.jobRepo.GetProgress(ctx, jobID)
	if err != nil {
		return fmt.Errorf("get progress: %w", err)
	}
	status := terminalStatus(succ, fail, blocked)
	startedAt, err := c.getStartedAt(ctx, jobID)
	if err != nil {
		return fmt.Errorf("get started_at: %w", err)
	}
	duration := time.Since(startedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	c.emitJobTerminal(ctx, jobID, status)
	return c.jobRepo.CompleteJob(ctx, jobID, status, succ, fail, blocked, duration)
}

// scrapeQueueDepth periodically inspects finance.cost.chunk and publishes the
// depth to the JobQueueDepth gauge. Failures are logged at debug level (the
// channel can momentarily be unhealthy on reconnect).
func (c *Coordinator) scrapeQueueDepth(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ch := c.rmqConn.Channel()
			if ch == nil {
				continue
			}
			q, err := ch.QueueInspect(rmq.QueueChunk)
			if err != nil {
				log.Debug().Err(err).Msg("queue inspect failed")
				continue
			}
			metrics.JobQueueDepth.Set(float64(q.Messages))
		}
	}
}

// emitJobTerminal increments finance_cost_jobs_total. Best-effort; jobID is
// only used to look up labels — if the lookup fails (e.g. row deleted) the
// counter is incremented with empty labels rather than skipped, so totals
// still match the actual transition count.
func (c *Coordinator) emitJobTerminal(ctx context.Context, jobID int64, status string) {
	job, err := c.jobRepo.GetByID(ctx, jobID)
	if err != nil || job == nil {
		metrics.JobsTotal.WithLabelValues(status, "", "", "").Inc()
		return
	}
	metrics.JobsTotal.WithLabelValues(
		status,
		string(job.CalcType),
		string(job.Scope),
		job.TriggeredBy,
	).Inc()
}

// terminalStatus picks SUCCESS / FAILED / PARTIAL_FAILED based on counters.
func terminalStatus(succ, fail, blocked int) string {
	switch {
	case fail == 0 && blocked == 0 && succ > 0:
		return statusSuccess
	case succ == 0:
		return statusFailed
	default:
		return statusPartial
	}
}

// getStartedAt is a thin SELECT helper used only at finalization time.
func (c *Coordinator) getStartedAt(ctx context.Context, jobID int64) (time.Time, error) {
	var started sql.NullTime
	const q = `SELECT COALESCE(cj_started_at, cj_queued_at) FROM cal_job WHERE cj_job_id = $1`
	if err := c.jobRepo.DB().QueryRowContext(ctx, q, jobID).Scan(&started); err != nil {
		return time.Time{}, fmt.Errorf("query started_at: %w", err)
	}
	if !started.Valid {
		return time.Now(), nil
	}
	return started.Time, nil
}

// getTotalWaves is a thin SELECT helper.
func (c *Coordinator) getTotalWaves(ctx context.Context, jobID int64) (int, error) {
	var total sql.NullInt32
	const q = `SELECT cj_total_waves FROM cal_job WHERE cj_job_id = $1`
	if err := c.jobRepo.DB().QueryRowContext(ctx, q, jobID).Scan(&total); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("query total_waves: %w", err)
	}
	return int(total.Int32), nil
}
