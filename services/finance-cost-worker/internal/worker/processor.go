// Package worker contains the RMQ -> finance.gRPC bridge. The worker consumes
// chunk messages from finance.cost.chunk, calls
// finance.CostCalcService/ProcessChunkInternal to compute, then publishes a
// ChunkCompletedEvent to finance.cost.chunk.completed. Retries (up to 3) are
// driven by RabbitMQ Nack-requeue; exhaustion DLQs the message and marks the
// chunk row FAILED.
package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/pkg/costcalc/metrics"
	"github.com/mutugading/goapps-backend/services/finance-cost-worker/internal/config"
	"github.com/mutugading/goapps-backend/services/finance-cost-worker/internal/infrastructure/financeclient"
	"github.com/mutugading/goapps-backend/services/finance-cost-worker/internal/infrastructure/rmq"
)

const (
	maxRetries          = 3
	statusSuccess       = "SUCCESS"
	statusPartialFailed = "PARTIAL_FAILED"
	statusFailed        = "FAILED"
)

// Worker is the RMQ -> gRPC bridge.
type Worker struct {
	cfg       *config.Config
	workerID  string
	consumer  *rmq.Consumer
	publisher *rmq.Publisher
	fin       *financeclient.Client
	chunks    *chunkRepo
}

// New constructs a Worker with all dependencies wired.
func New(cfg *config.Config, workerID string, db *sql.DB, rmqConn *rmq.Connection, fin *financeclient.Client) *Worker {
	return &Worker{
		cfg:       cfg,
		workerID:  workerID,
		consumer:  rmq.NewConsumer(rmqConn, rmq.QueueChunk, "worker-"+workerID),
		publisher: rmq.NewPublisher(rmqConn),
		fin:       fin,
		chunks:    newChunkRepo(db),
	}
}

// Run blocks consuming chunks until ctx is canceled.
func (w *Worker) Run(ctx context.Context) error {
	log.Info().Str("worker_id", w.workerID).Msg("worker consume loop starting")
	if err := w.consumer.Consume(ctx, w.handleChunk); err != nil {
		return fmt.Errorf("consume loop: %w", err)
	}
	log.Info().Str("worker_id", w.workerID).Msg("worker consume loop stopped")
	return nil
}

// handleChunk is the per-message workflow:
//  1. parse JSON -> ChunkMessage
//  2. idempotency check: if chunk row is already terminal, ack + republish event
//  3. mark chunk PROCESSING
//  4. call finance gRPC; on transport / response failure run retry path
//  5. on success: mark chunk completed + publish ChunkCompletedEvent + ack
func (w *Worker) handleChunk(ctx context.Context, d amqp.Delivery) error {
	metrics.WorkerActiveChunks.WithLabelValues(w.workerID).Inc()
	defer metrics.WorkerActiveChunks.WithLabelValues(w.workerID).Dec()

	var msg ChunkMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		log.Error().Err(err).Bytes("body", d.Body).Msg("malformed chunk; nacking to DLQ")
		if nackErr := d.Nack(false, false); nackErr != nil {
			return fmt.Errorf("nack malformed: %w", nackErr)
		}
		return nil
	}

	// Continue the orchestrator's trace: extract the propagated context from the
	// AMQP headers, then open the per-chunk span as a child of the job span. The
	// span's context is threaded into the gRPC call so finance continues the
	// trace. When tracing is disabled extraction yields the no-op context and
	// span creation costs nothing.
	parentCtx := otel.GetTextMapPropagator().Extract(ctx, propagation.TextMapCarrier(rmq.HeaderCarrier(d.Headers)))
	ctx, span := otel.Tracer(tracerName).Start(parentCtx, spanCostCalcChunk, trace.WithSpanKind(trace.SpanKindConsumer))
	defer span.End()
	span.SetAttributes(
		attribute.Int64("chunk_id", msg.ChunkID),
		attribute.Int64("job_id", msg.JobID),
		attribute.Int("wave_no", msg.WaveNo),
		attribute.Int("product_count", len(msg.ProductIDs)),
		attribute.String("worker_id", w.workerID),
	)

	if cur, err := w.chunks.GetStatus(ctx, msg.ChunkID); err == nil && isTerminal(cur) {
		log.Info().Int64("chunk_id", msg.ChunkID).Str("status", cur).Msg("duplicate delivery for terminal chunk; acking")
		if pubErr := w.publishCompletion(ctx, msg, cur, 0, 0, 0); pubErr != nil {
			log.Warn().Err(pubErr).Int64("chunk_id", msg.ChunkID).Msg("republish completion failed")
		}
		return d.Ack(false)
	}

	start := time.Now()
	log.Info().
		Int64("chunk_id", msg.ChunkID).
		Int64("job_id", msg.JobID).
		Int("products", len(msg.ProductIDs)).
		Msg("processing chunk")

	if err := w.chunks.MarkProcessing(ctx, msg.ChunkID, w.workerID); err != nil {
		log.Error().Err(err).Int64("chunk_id", msg.ChunkID).Msg("mark processing failed; requeueing")
		return d.Nack(false, true)
	}

	resp, err := w.fin.ProcessChunk(ctx, &financev1.ProcessChunkInternalRequest{
		JobId:           msg.JobID,
		ChunkId:         msg.ChunkID,
		Period:          msg.Period,
		CalculationType: parseCalcType(msg.CalculationType),
		ProductIds:      msg.ProductIDs,
		Actor:           msg.Actor,
	})
	if err != nil {
		return w.handleFailure(ctx, d, msg, fmt.Errorf("grpc call: %w", err))
	}
	if !resp.GetBase().GetIsSuccess() {
		return w.handleFailure(ctx, d, msg, fmt.Errorf("finance error: %s", resp.GetBase().GetMessage()))
	}

	elapsed := time.Since(start)
	durationMs := int(elapsed.Milliseconds())
	metrics.ChunkDurationSeconds.WithLabelValues(strconv.Itoa(msg.WaveNo)).Observe(elapsed.Seconds())
	succ, fail, blocked := int(resp.GetSuccessCount()), int(resp.GetFailedCount()), int(resp.GetBlockedCount())
	status := classify(succ, fail, blocked)

	if err := w.chunks.MarkCompleted(ctx, msg.ChunkID, status, succ, fail, durationMs); err != nil {
		log.Error().Err(err).Int64("chunk_id", msg.ChunkID).Msg("mark completed failed; will requeue")
		return d.Nack(false, true)
	}

	if err := w.publishCompletion(ctx, msg, status, succ, fail, blocked); err != nil {
		// Status row IS persisted. Orchestrator can sweep stuck chunks; do not requeue.
		log.Error().Err(err).Int64("chunk_id", msg.ChunkID).Msg("publish completion failed; row IS persisted")
	}

	log.Info().
		Int64("chunk_id", msg.ChunkID).
		Str("status", status).
		Int("succ", succ).
		Int("fail", fail).
		Int("blocked", blocked).
		Int("ms", durationMs).
		Msg("chunk done")
	return d.Ack(false)
}

// handleFailure runs the retry path. Increments the chunk's retry counter; if
// under the limit Nack-requeues so RMQ redelivers (possibly to another worker);
// otherwise marks FAILED + DLQs.
func (w *Worker) handleFailure(ctx context.Context, d amqp.Delivery, msg ChunkMessage, cause error) error {
	n, err := w.chunks.IncrementRetry(ctx, msg.ChunkID)
	if err != nil {
		log.Error().Err(err).Int64("chunk_id", msg.ChunkID).Msg("increment retry failed; nacking to DLQ")
		return d.Nack(false, false)
	}
	if n < maxRetries {
		log.Warn().Err(cause).Int("attempt", n).Int64("chunk_id", msg.ChunkID).Msg("requeueing for retry")
		return d.Nack(false, true)
	}
	log.Error().Err(cause).Int64("chunk_id", msg.ChunkID).Msg("max retries exhausted; marking FAILED + DLQ")
	if markErr := w.chunks.MarkCompleted(ctx, msg.ChunkID, statusFailed, 0, len(msg.ProductIDs), 0); markErr != nil {
		log.Error().Err(markErr).Int64("chunk_id", msg.ChunkID).Msg("mark FAILED after retry exhaustion")
	}
	if pubErr := w.publishCompletion(ctx, msg, statusFailed, 0, len(msg.ProductIDs), 0); pubErr != nil {
		log.Error().Err(pubErr).Int64("chunk_id", msg.ChunkID).Msg("publish FAILED completion")
	}
	return d.Nack(false, false)
}

func (w *Worker) publishCompletion(ctx context.Context, msg ChunkMessage, status string, succ, fail, blocked int) error {
	ev := ChunkCompletedEvent{
		ChunkID:      msg.ChunkID,
		JobID:        msg.JobID,
		WaveNo:       msg.WaveNo,
		Status:       status,
		SuccessCount: succ,
		FailedCount:  fail,
		BlockedCount: blocked,
	}
	return w.publisher.Publish(ctx, rmq.RoutingKeyChunkDone, ev)
}

func isTerminal(status string) bool {
	return status == statusSuccess || status == statusFailed || status == statusPartialFailed
}

func classify(succ, fail, blocked int) string {
	switch {
	case fail == 0 && blocked == 0:
		return statusSuccess
	case succ == 0:
		return statusFailed
	default:
		return statusPartialFailed
	}
}

func parseCalcType(s string) financev1.CalculationType {
	switch s {
	case "ACTUAL":
		return financev1.CalculationType_CALCULATION_TYPE_ACTUAL
	case "FORECAST":
		return financev1.CalculationType_CALCULATION_TYPE_FORECAST
	case "SELLING":
		return financev1.CalculationType_CALCULATION_TYPE_SELLING
	default:
		return financev1.CalculationType_CALCULATION_TYPE_UNSPECIFIED
	}
}
