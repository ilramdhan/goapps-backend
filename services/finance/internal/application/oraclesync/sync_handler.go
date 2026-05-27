// Package oraclesync provides application-level orchestration for Oracle-to-PostgreSQL sync jobs.
package oraclesync

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
)

const (
	// OracleSchema is the Oracle schema containing the stored procedure.
	OracleSchema = "MGTDAT"
	// OracleProcedure is the stored procedure that refreshes Oracle data.
	OracleProcedure = "PRC_CST_CONSSTKPO_MGT"

	stepProcedure = "execute_procedure"
	stepFetch     = "fetch_data"
	stepUpsert    = "upsert_data"
)

// ChainPublisher publishes a follow-up RM cost calculation job after a
// successful Oracle sync. Matches the rabbitmq.JobPublisherAdapter method set.
// Nil is allowed — chaining is then skipped (useful for tests / dry runs).
type ChainPublisher interface {
	PublishRMCostCalculation(ctx context.Context, jobID, period string, groupHeadID *uuid.UUID, reason, createdBy string) error
}

// SyncHandler orchestrates the Oracle sync process.
type SyncHandler struct {
	jobRepo    job.Repository
	oracleRepo syncdata.OracleSourceRepository
	pgRepo     syncdata.PostgresTargetRepository
	chainPub   ChainPublisher
	logger     zerolog.Logger
}

// NewSyncHandler creates a new SyncHandler.
func NewSyncHandler(
	jobRepo job.Repository,
	oracleRepo syncdata.OracleSourceRepository,
	pgRepo syncdata.PostgresTargetRepository,
	logger zerolog.Logger,
) *SyncHandler {
	return &SyncHandler{
		jobRepo:    jobRepo,
		oracleRepo: oracleRepo,
		pgRepo:     pgRepo,
		logger:     logger,
	}
}

// WithChainPublisher installs the follow-up publisher so that a successful sync
// automatically enqueues an RM cost recalculation for the synced period.
func (h *SyncHandler) WithChainPublisher(pub ChainPublisher) *SyncHandler {
	h.chainPub = pub
	return h
}

// Execute runs the full sync workflow for a given job.
func (h *SyncHandler) Execute(ctx context.Context, jobID uuid.UUID) error {
	// Fetch the job execution.
	exec, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("get job %s: %w", jobID, err)
	}

	// If the job was canceled or already completed before the worker picked it up, ACK silently.
	if exec.Status().IsTerminal() {
		h.logger.Info().
			Str("job_id", jobID.String()).
			Str("status", exec.Status().String()).
			Msg("Skipping job — already in terminal state")
		return nil
	}

	// Transition to processing.
	if err := exec.Start(); err != nil {
		return fmt.Errorf("start job %s: %w", jobID, err)
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		return fmt.Errorf("update status to processing: %w", err)
	}

	h.logger.Info().
		Str("job_id", jobID.String()).
		Str("period", exec.Period()).
		Msg("Starting Oracle sync job")

	// Run the sync steps. On any failure, mark the job as failed.
	if syncErr := h.runSync(ctx, exec); syncErr != nil {
		h.logger.Error().Err(syncErr).
			Str("job_id", jobID.String()).
			Msg("Sync job failed")
		return h.failJob(ctx, exec, syncErr)
	}

	return nil
}

func (h *SyncHandler) runSync(ctx context.Context, exec *job.Execution) error {
	period := exec.Period()
	jobID := exec.ID()

	// Step 1: Execute Oracle stored procedure.
	if err := h.executeProcedure(ctx, jobID, period); err != nil {
		return err
	}
	if err := h.jobRepo.UpdateProgress(ctx, jobID, 30); err != nil {
		h.logger.Warn().Err(err).Msg("Failed to update progress to 30%")
	}

	// Step 2: Fetch data from Oracle.
	items, err := h.fetchData(ctx, jobID, period)
	if err != nil {
		return err
	}
	if err := h.jobRepo.UpdateProgress(ctx, jobID, 60); err != nil {
		h.logger.Warn().Err(err).Msg("Failed to update progress to 60%")
	}

	// Step 3: Upsert to PostgreSQL.
	result, err := h.upsertData(ctx, jobID, items)
	if err != nil {
		return err
	}
	if err := h.jobRepo.UpdateProgress(ctx, jobID, 100); err != nil {
		h.logger.Warn().Err(err).Msg("Failed to update progress to 100%")
	}

	// Complete the job with result summary.
	if err := h.completeJob(ctx, exec, result); err != nil {
		return err
	}

	// Chain-trigger RM cost recalculation for the synced period. Failure here
	// does not fail the sync job — operators can re-run cost calc manually.
	h.publishCostChain(ctx, exec.Period(), exec.CreatedBy())
	return nil
}

func (h *SyncHandler) publishCostChain(ctx context.Context, period, createdBy string) {
	if h.chainPub == nil {
		return
	}

	chainExec, err := job.NewExecution(job.TypeRMCostCalculation, "landed_cost", period, createdBy, 5, nil)
	if err != nil {
		h.logger.Warn().Err(err).Str("period", period).Msg("Failed to build chained rm cost job")
		return
	}
	if err := h.jobRepo.Create(ctx, chainExec); err != nil {
		h.logger.Warn().Err(err).Str("period", period).Msg("Failed to persist chained rm cost job")
		return
	}

	if err := h.chainPub.PublishRMCostCalculation(ctx, chainExec.ID().String(), period, nil, "oracle-sync-chain", createdBy); err != nil {
		// Compensate: mark the created job as failed so it doesn't linger as queued.
		if failErr := chainExec.Fail(err.Error()); failErr == nil {
			if updErr := h.jobRepo.UpdateStatus(ctx, chainExec); updErr != nil {
				h.logger.Warn().Err(updErr).Msg("Failed to mark chained job as failed")
			}
		}
		h.logger.Warn().Err(err).
			Str("period", period).
			Msg("Failed to chain-trigger RM cost calculation (operator can run manually)")
		return
	}
	h.logger.Info().
		Str("chain_job_id", chainExec.ID().String()).
		Str("period", period).
		Msg("Chained RM cost calculation job enqueued")
}

func (h *SyncHandler) executeProcedure(ctx context.Context, jobID uuid.UUID, period string) error {
	logEntry := job.NewExecutionLog(jobID, stepProcedure, job.LogStarted,
		fmt.Sprintf("Executing %s.%s (auto-period from SYSDATE, requested: %s)", OracleSchema, OracleProcedure, period), nil)
	if err := h.jobRepo.AddLog(ctx, logEntry); err != nil {
		h.logger.Warn().Err(err).Msg("Failed to add procedure log")
	}

	start := time.Now()
	err := h.oracleRepo.ExecuteProcedure(ctx, OracleSchema, OracleProcedure)
	duration := time.Since(start)

	if err != nil {
		logEntry.MarkCompleted(job.LogFailed, fmt.Sprintf("Procedure failed after %s: %v", duration, err))
		if logErr := h.jobRepo.UpdateLog(ctx, logEntry); logErr != nil {
			h.logger.Warn().Err(logErr).Msg("Failed to update procedure failure log")
		}
		return fmt.Errorf("%w: %w", syncdata.ErrProcedureFailed, err)
	}

	logEntry.MarkCompleted(job.LogSuccess, fmt.Sprintf("Procedure completed in %s", duration))
	if logErr := h.jobRepo.UpdateLog(ctx, logEntry); logErr != nil {
		h.logger.Warn().Err(logErr).Msg("Failed to update procedure success log")
	}

	h.logger.Info().
		Dur("duration", duration).
		Str("period", period).
		Msg("Oracle procedure completed")

	return nil
}

func (h *SyncHandler) fetchData(ctx context.Context, jobID uuid.UUID, period string) ([]*syncdata.ItemConsStockPO, error) {
	logEntry := job.NewExecutionLog(jobID, stepFetch, job.LogStarted,
		fmt.Sprintf("Fetching data for period %s from Oracle", period), nil)
	if err := h.jobRepo.AddLog(ctx, logEntry); err != nil {
		h.logger.Warn().Err(err).Msg("Failed to add fetch log")
	}

	start := time.Now()
	items, err := h.oracleRepo.FetchItemConsStockPO(ctx, period)
	duration := time.Since(start)

	if err != nil {
		logEntry.MarkCompleted(job.LogFailed, fmt.Sprintf("Fetch failed after %s: %v", duration, err))
		if logErr := h.jobRepo.UpdateLog(ctx, logEntry); logErr != nil {
			h.logger.Warn().Err(logErr).Msg("Failed to update fetch failure log")
		}
		return nil, fmt.Errorf("%w: %w", syncdata.ErrFetchFailed, err)
	}

	logEntry.MarkCompleted(job.LogSuccess, fmt.Sprintf("Fetched %d rows in %s", len(items), duration))
	if logErr := h.jobRepo.UpdateLog(ctx, logEntry); logErr != nil {
		h.logger.Warn().Err(logErr).Msg("Failed to update fetch success log")
	}

	h.logger.Info().
		Int("rows", len(items)).
		Dur("duration", duration).
		Str("period", period).
		Msg("Oracle data fetch completed")

	return items, nil
}

func (h *SyncHandler) upsertData(ctx context.Context, jobID uuid.UUID, items []*syncdata.ItemConsStockPO) (*syncdata.UpsertResult, error) {
	logEntry := job.NewExecutionLog(jobID, stepUpsert, job.LogStarted,
		fmt.Sprintf("Upserting %d rows to PostgreSQL", len(items)), nil)
	if err := h.jobRepo.AddLog(ctx, logEntry); err != nil {
		h.logger.Warn().Err(err).Msg("Failed to add upsert log")
	}

	start := time.Now()
	result, err := h.pgRepo.UpsertItemConsStockPO(ctx, items, jobID)
	duration := time.Since(start)

	if err != nil {
		logEntry.MarkCompleted(job.LogFailed, fmt.Sprintf("Upsert failed after %s: %v", duration, err))
		if logErr := h.jobRepo.UpdateLog(ctx, logEntry); logErr != nil {
			h.logger.Warn().Err(logErr).Msg("Failed to update upsert failure log")
		}
		return nil, fmt.Errorf("%w: %w", syncdata.ErrUpsertFailed, err)
	}

	logEntry.MarkCompleted(job.LogSuccess,
		fmt.Sprintf("Upserted %d rows (%d inserted, %d updated) in %s",
			result.TotalRows, result.Inserted, result.Updated, duration))
	if logErr := h.jobRepo.UpdateLog(ctx, logEntry); logErr != nil {
		h.logger.Warn().Err(logErr).Msg("Failed to update upsert success log")
	}

	h.logger.Info().
		Int("total", result.TotalRows).
		Int("inserted", result.Inserted).
		Int("updated", result.Updated).
		Dur("duration", duration).
		Msg("PostgreSQL upsert completed")

	return result, nil
}

func (h *SyncHandler) completeJob(ctx context.Context, exec *job.Execution, result *syncdata.UpsertResult) error {
	summary, err := json.Marshal(map[string]any{
		"total_rows": result.TotalRows,
		"inserted":   result.Inserted,
		"updated":    result.Updated,
		"period":     exec.Period(),
	})
	if err != nil {
		return fmt.Errorf("marshal result summary: %w", err)
	}

	if err := exec.Complete(summary); err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		return fmt.Errorf("update status to success: %w", err)
	}

	h.logger.Info().
		Str("job_id", exec.ID().String()).
		Str("code", exec.Code().String()).
		Msg("Oracle sync job completed successfully")

	return nil
}

func (h *SyncHandler) failJob(ctx context.Context, exec *job.Execution, syncErr error) error {
	if failErr := exec.Fail(syncErr.Error()); failErr != nil {
		h.logger.Error().Err(failErr).Msg("Failed to transition job to failed state")
		return fmt.Errorf("fail job: %w (original: %w)", failErr, syncErr)
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Error().Err(err).Msg("Failed to persist job failure status")
		return fmt.Errorf("update status to failed: %w (original: %w)", err, syncErr)
	}
	return syncErr
}

// ResolvePeriod returns the sync period based on the current date.
// Matches Oracle procedure logic: Day 1-5: previous month, Day 6+: current month.
// Format: YYYYMM (e.g., "202601").
func ResolvePeriod(now time.Time) string {
	if now.Day() <= 5 {
		prev := now.AddDate(0, -1, 0)
		return prev.Format("200601")
	}
	return now.Format("200601")
}
